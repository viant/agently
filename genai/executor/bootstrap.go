package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	neturl "net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"

	clientmcp "github.com/viant/agently/adapter/mcp"
	"github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/elicitation"
	elicrouter "github.com/viant/agently/genai/elicitation/router"
	embedderprovider "github.com/viant/agently/genai/embedder/provider"
	llmprovider "github.com/viant/agently/genai/llm/provider"
	gtool "github.com/viant/agently/genai/tool"
	"github.com/viant/agently/internal/workspace/repository/agent"
	embedderrepo "github.com/viant/agently/internal/workspace/repository/embedder"
	extrepo "github.com/viant/agently/internal/workspace/repository/extension"
	"github.com/viant/agently/internal/workspace/repository/model"

	"github.com/viant/afs"
	"github.com/viant/agently/internal/workspace"
	mcprepo "github.com/viant/agently/internal/workspace/repository/mcp"
	"github.com/viant/datly/view"
	// decoupled from orchestration
	mcpcfg "github.com/viant/agently/internal/mcp/config"
	mcpmgr "github.com/viant/agently/internal/mcp/manager"
	protoclient "github.com/viant/mcp-protocol/client"
	// mcp client types not used here
	"gopkg.in/yaml.v3"

	"github.com/viant/agently/genai/conversation"
	"github.com/viant/agently/genai/memory"
	agent2 "github.com/viant/agently/genai/service/agent"
	augmenter "github.com/viant/agently/genai/service/augmenter"
	core "github.com/viant/agently/genai/service/core"
	llmagents "github.com/viant/agently/genai/tool/service/llm/agents"
	llmexectool "github.com/viant/agently/genai/tool/service/llm/exec"
	msgsvc "github.com/viant/agently/genai/tool/service/message"
	// External A2A client
	a2acli "github.com/viant/a2a-protocol/client"
	a2aschema "github.com/viant/a2a-protocol/schema"
	a2asrv "github.com/viant/a2a-protocol/server"
	aauth "github.com/viant/a2a-protocol/server/auth"
	jsonrpc "github.com/viant/jsonrpc"
	// removed executor options
	// Helpers for exposing agents as tools
	//	"github.com/viant/agently/genai/executor/agenttool"
	apprmem "github.com/viant/agently/internal/approval/memory"
)

// init prepares the Service for handling requests.
func (e *Service) init(ctx context.Context) error {

	// ------------------------------------------------------------------
	// Step 1: defaults & validation
	// ------------------------------------------------------------------
	e.initDefaults(ctx)
	if err := e.config.Validate(); err != nil {
		return err
	}

	// Validate extension feeds strictly: abort startup when any feed is invalid.
	if err := e.validateExtensions(ctx); err != nil {
		return err
	}

	// ------------------------------------------------------------------
	// Step 3: Tool registry (MCP-backed) and agent tools exposure
	// ------------------------------------------------------------------
	if e.tools == nil {
		if e.mcpMgr == nil {
			return fmt.Errorf("executor: mcp manager not configured for tool registry")
		}
		reg, err := gtool.NewDefaultRegistry(e.mcpMgr)
		if err != nil {
			return err
		}
		// Eagerly initialize registry (preload MCP servers/tools). Warnings are logged.
		reg.Initialize(ctx)
		// Expose internal agents as virtual tools driven by Profile.Publish
		if e.config != nil && e.config.Agent != nil && len(e.config.Agent.Items) > 0 {
			gtool.InjectVirtualAgentTools(reg, e.config.Agent.Items, "")
		}
		e.tools = reg
	}

	// Initialise decoupled core/agent services and conversation manager
	enricher := augmenter.New(e.embedderFinder, augmenter.WithMCPManager(e.mcpMgr))
	e.llmCore = core.New(e.modelFinder, e.tools, e.convClient)
	agentSvc := agent2.New(e.llmCore, e.agentFinder, enricher, e.tools, &e.config.Default, e.convClient,
		func(s *agent2.Service) {
			if e.elicitationRouter != nil {
				agent2.WithElicitationRouter(e.elicitationRouter)(s)
			}
		},
		func(s *agent2.Service) {
			if e.newAwaiter != nil {
				agent2.WithNewElicitationAwaiter(e.newAwaiter)(s)
			}
		},
	)
	e.agentService = agentSvc

	// Register llm/exec service (legacy run_agent) and new llm/agents service.
	gtool.AddInternalService(e.tools, llmexectool.New(agentSvc))
	// Load external A2A agents from workspace a2a/ folder
	type extSpec struct {
		ID         string            `yaml:"id"`
		Enabled    *bool             `yaml:"enabled,omitempty"`
		JSONRPCURL string            `yaml:"jsonrpcURL"`
		StreamURL  string            `yaml:"streamURL,omitempty"`
		Headers    map[string]string `yaml:"headers,omitempty"`
		Directory  struct {
			Name        string   `yaml:"name,omitempty"`
			Description string   `yaml:"description,omitempty"`
			Tags        []string `yaml:"tags,omitempty"`
			Priority    int      `yaml:"priority,omitempty"`
		} `yaml:"directory,omitempty"`
	}
	extRoutes := map[string]extSpec{}
	var extErrors []string
	if e.config != nil {
		if names, _ := e.config.Meta().List(ctx, "a2a"); len(names) > 0 {
			for _, u := range names {
				var spec extSpec
				if err := e.config.Meta().Load(ctx, u, &spec); err != nil {
					extErrors = append(extErrors, fmt.Sprintf("load %s failed: %v", u, err))
					continue
				}
				if strings.TrimSpace(spec.ID) == "" || strings.TrimSpace(spec.JSONRPCURL) == "" {
					extErrors = append(extErrors, fmt.Sprintf("invalid spec (missing id or jsonrpcURL) at %s", u))
					continue
				}
				if spec.Enabled != nil && !*spec.Enabled {
					continue
				}
				id := strings.TrimSpace(spec.ID)
				// Validate URLs
				if p, err := neturl.Parse(spec.JSONRPCURL); err != nil || (p.Scheme != "http" && p.Scheme != "https") {
					extErrors = append(extErrors, fmt.Sprintf("invalid jsonrpcURL for %s: %s", id, spec.JSONRPCURL))
					continue
				}
				if strings.TrimSpace(spec.StreamURL) != "" {
					if p, err := neturl.Parse(spec.StreamURL); err != nil || (p.Scheme != "http" && p.Scheme != "https") {
						extErrors = append(extErrors, fmt.Sprintf("invalid streamURL for %s: %s", id, spec.StreamURL))
						continue
					}
				}
				if _, exists := extRoutes[id]; exists {
					extErrors = append(extErrors, fmt.Sprintf("duplicate external id %s – keeping first, ignoring %s", id, u))
					continue
				}
				extRoutes[id] = spec
			}
		}
		if len(extErrors) > 0 {
			for _, m := range extErrors {
				log.Printf("a2a: %s", m)
			}
		}
	}

	// Build allowed routing map: ids present in directory (internal enabled + external routes not shadowed)
	allowed := map[string]string{}
	if e.config != nil && e.config.Agent != nil {
		for _, a := range e.config.Agent.Items {
			if a == nil || strings.TrimSpace(a.ID) == "" {
				continue
			}
			if a.Profile == nil || !a.Profile.Publish {
				continue
			}
			allowed[strings.TrimSpace(a.ID)] = "internal"
		}
	}
	for id := range extRoutes {
		if _, ok := allowed[id]; ok {
			continue
		}
		allowed[id] = "external"
	}

	// Directory provider: merge internal (enabled) agents and external A2A entries; internal takes priority on id conflict
	dirProvider := func() []llmagents.ListItem {
		var items []llmagents.ListItem
		seen := map[string]struct{}{}
		if e.config != nil && e.config.Agent != nil {
			for _, a := range e.config.Agent.Items {
				if a == nil || strings.TrimSpace(a.ID) == "" {
					continue
				}
				if a.Profile == nil || !a.Profile.Publish {
					continue
				}
				id := strings.TrimSpace(a.ID)
				name := strings.TrimSpace(a.Profile.Name)
				if name == "" {
					name = strings.TrimSpace(a.Name)
				}
				if name == "" {
					name = id
				}
				desc := strings.TrimSpace(a.Profile.Description)
				if desc == "" {
					desc = strings.TrimSpace(a.Description)
				}
				var caps map[string]interface{}
				if a.Profile != nil && a.Profile.Capabilities != nil {
					caps = a.Profile.Capabilities
				}
				var tags []string
				if a.Profile != nil && len(a.Profile.Tags) > 0 {
					tags = append([]string(nil), a.Profile.Tags...)
				}
				rank := a.Profile.Rank
				items = append(items, llmagents.ListItem{
					ID:           id,
					Name:         name,
					Description:  desc,
					Tags:         tags,
					Priority:     rank,
					Capabilities: caps,
					Source:       "internal",
				})
				seen[id] = struct{}{}
			}
		}
		for id, s := range extRoutes {
			if _, ok := seen[id]; ok {
				log.Printf("a2a: external id %q conflicts with internal; internal takes priority", id)
				continue
			}
			name := strings.TrimSpace(s.Directory.Name)
			if name == "" {
				name = id
			}
			items = append(items, llmagents.ListItem{
				ID:           id,
				Name:         name,
				Description:  strings.TrimSpace(s.Directory.Description),
				Tags:         append([]string(nil), s.Directory.Tags...),
				Priority:     s.Directory.Priority,
				Capabilities: nil,
				Source:       "external",
			})
		}
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].Priority == items[j].Priority {
				return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
			}
			return items[i].Priority > items[j].Priority
		})
		return items
	}

	// External run resolver in-process
	runExternal := func(ctx context.Context, agentID, objective string, payload map[string]interface{}) (answer, status, taskID, contextID string, streamSupported bool, warnings []string, err error) {
		spec, ok := extRoutes[strings.TrimSpace(agentID)]
		if !ok {
			return
		}
		cli := a2acli.New(strings.TrimSpace(spec.JSONRPCURL))
		for k, v := range spec.Headers {
			cli.Headers.Set(k, v)
		}

		// Build message using objective; context payload not mapped yet.
		msg := a2aschema.Message{Role: a2aschema.RoleUser, Parts: []a2aschema.Part{a2aschema.TextPart{Type: "text", Text: objective}}}

		// Use current conversation as contextId when available
		ctxID := memory.ConversationIDFromContext(ctx)
		var ctxPtr *string
		if strings.TrimSpace(ctxID) != "" {
			ctxPtr = &ctxID
		}

		// Respect a per-call timeout when the context has no deadline
		cctx := ctx
		if _, ok := ctx.Deadline(); !ok {
			timeout := time.Duration(120) * time.Second
			if e.config != nil && e.config.A2ATimeoutSec > 0 {
				timeout = time.Duration(e.config.A2ATimeoutSec) * time.Second
			}
			var cancel context.CancelFunc
			cctx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}
		task, callErr := cli.SendMessage(cctx, []a2aschema.Message{msg}, ctxPtr)
		if callErr != nil {
			err = callErr
			return
		}
		// Extract answer from first text artifact if present
		for _, art := range task.Artifacts {
			for _, p := range art.Parts {
				if tp, ok := p.(a2aschema.TextPart); ok && strings.TrimSpace(tp.Text) != "" {
					answer = strings.TrimSpace(tp.Text)
					break
				}
			}
			if answer != "" {
				break
			}
		}
		taskID = task.ID
		if task.ContextID != nil {
			contextID = *task.ContextID
		}
		status = string(task.Status.State)
		streamSupported = strings.TrimSpace(spec.StreamURL) != ""
		return
	}

	// Strict routing default true; allow override via config.Directory.Strict
	strict := true
	if e.config != nil && e.config.Directory != nil && e.config.Directory.Strict != nil {
		strict = *e.config.Directory.Strict
	}

	// New consolidated directory/run facade with external routing hook and routing policy
	gtool.AddInternalService(e.tools, llmagents.New(
		agentSvc,
		llmagents.WithDirectoryProvider(dirProvider),
		llmagents.WithExternalRunner(runExternal),
		llmagents.WithAllowedIDs(allowed),
		llmagents.WithStrict(strict),
		llmagents.WithConversationClient(e.convClient),
	))
	// Expose configured internal agents as A2A servers
	if e.config != nil && e.config.Agent != nil {
		for _, a := range e.config.Agent.Items {
			// Prefer Serve.A2A; fallback to legacy ExposeA2A
			var a2aEnabled bool
			var a2aPort int
			if a != nil && a.Serve != nil && a.Serve.A2A != nil && a.Serve.A2A.Enabled && a.Serve.A2A.Port > 0 {
				a2aEnabled = true
				a2aPort = a.Serve.A2A.Port
			} else if a != nil && a.ExposeA2A != nil && a.ExposeA2A.Enabled && a.ExposeA2A.Port > 0 {
				a2aEnabled = true
				a2aPort = a.ExposeA2A.Port
			}
			if !a2aEnabled {
				continue
			}
			// local copy for closure
			ag := a
			go func() {
				base := "/v1"
				addr := fmt.Sprintf(":%d", a2aPort)
				// Build AgentCard
				name := strings.TrimSpace(ag.Name)
				if name == "" {
					name = strings.TrimSpace(ag.ID)
				}
				desc := strings.TrimSpace(ag.Description)
				var descPtr *string
				if desc != "" {
					descPtr = &desc
				}
				card := a2aschema.AgentCard{Name: name, Description: descPtr}
				// Prefer Serve.A2A when present
				sFlag := false
				if ag.Serve != nil && ag.Serve.A2A != nil {
					sFlag = ag.Serve.A2A.Streaming
				} else if ag.ExposeA2A != nil {
					sFlag = ag.ExposeA2A.Streaming
				}
				card.SetCapabilities(a2aschema.AgentCapabilities{Streaming: &sFlag})
				// Handlers mapping A2A message/send into internal agent runtime
				newOps := a2asrv.WithDefaultHandler(context.Background(),
					a2asrv.RegisterMessageSend(func(ctx context.Context, messages []a2aschema.Message, contextID, taskID *string) (*a2aschema.Task, *jsonrpc.Error) {
						// Collect objective from text parts
						objective := ""
						for _, m := range messages {
							for _, p := range m.Parts {
								if tp, ok := p.(a2aschema.TextPart); ok {
									if s := strings.TrimSpace(tp.Text); s != "" {
										if objective != "" {
											objective += "\n"
										}
										objective += s
									}
								}
							}
						}
						qi := &agent2.QueryInput{AgentID: ag.ID, Query: objective}
						if contextID != nil && strings.TrimSpace(*contextID) != "" {
							qi.ConversationID = *contextID
						}
						var qo agent2.QueryOutput
						if err := agentSvc.Query(context.Background(), qi, &qo); err != nil {
							return nil, jsonrpc.NewError(-32000, err.Error(), nil)
						}
						// Build completed task with a text artifact
						t := &a2aschema.Task{ID: "t-" + qo.MessageID}
						art := a2aschema.Artifact{ID: "a-" + t.ID, CreatedAt: time.Now().UTC(), Parts: []a2aschema.Part{a2aschema.TextPart{Type: "text", Text: qo.Content}}}
						art.PartsRaw, _ = a2aschema.MarshalParts(art.Parts)
						t.Status = a2aschema.TaskStatus{State: a2aschema.TaskCompleted, UpdatedAt: time.Now().UTC()}
						t.Artifacts = []a2aschema.Artifact{art}
						return t, nil
					}),
					a2asrv.RegisterMessageStream(func(ctx context.Context, messages []a2aschema.Message, contextID, taskID *string) (*a2aschema.Task, *jsonrpc.Error) {
						return nil, jsonrpc.NewMethodNotFound("message/stream not supported", nil)
					}),
				)
				srv := a2asrv.New(card, a2asrv.WithOperations(newOps))
				inner := http.NewServeMux()
				srv.RegisterSSE(inner, base)
				srv.RegisterStreaming(inner, "/a2a")
				srv.RegisterREST(inner)
				outer := http.NewServeMux()
				outer.HandleFunc("/.well-known/agent-card.json", func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(card)
				})
				var authCfg *A2AAuthProxy
				if ag.Serve != nil && ag.Serve.A2A != nil && ag.Serve.A2A.Auth != nil && ag.Serve.A2A.Auth.Enabled {
					a := ag.Serve.A2A.Auth
					authCfg = &A2AAuthProxy{Enabled: a.Enabled, Resource: a.Resource, Scopes: a.Scopes, UseIDToken: a.UseIDToken, ExcludePrefix: a.ExcludePrefix}
				} else if ag.ExposeA2A != nil && ag.ExposeA2A.Auth != nil && ag.ExposeA2A.Auth.Enabled {
					a := ag.ExposeA2A.Auth
					authCfg = &A2AAuthProxy{Enabled: a.Enabled, Resource: a.Resource, Scopes: a.Scopes, UseIDToken: a.UseIDToken, ExcludePrefix: a.ExcludePrefix}
				}
				if authCfg != nil {
					pol := &aauth.Policy{Metadata: &aauth.ProtectedResourceMetadata{Resource: authCfg.Resource, ScopesSupported: authCfg.Scopes}, UseIDToken: authCfg.UseIDToken}
					pol.ExcludePrefix = strings.TrimSpace(authCfg.ExcludePrefix)
					authSvc := aauth.NewService(pol)
					authSvc.RegisterHandlers(outer)
					outer.Handle("/", authSvc.Middleware(inner))
				} else {
					outer.Handle("/", inner)
				}
				log.Printf("A2A agent '%s' on %s (base %s)", name, addr, base)
				_ = http.ListenAndServe(addr, outer)
			}()
		}
	}
	// Register internal message service (unified show/summarize/match/remove)
	summarizeChunk := 4096
	matchChunk := 1024
	summaryModel := ""
	summaryPrompt := ""
	embedModel := ""
	defaultModel := ""
	if e.config != nil {
		if e.config.Default.ToolCallResult.SummarizeChunk > 0 {
			summarizeChunk = e.config.Default.ToolCallResult.SummarizeChunk
		}
		if e.config.Default.ToolCallResult.MatchChunk > 0 {
			matchChunk = e.config.Default.ToolCallResult.MatchChunk
		}
		summaryModel = e.config.Default.ToolCallResult.SummaryModel
		if strings.TrimSpace(summaryModel) == "" {
			summaryModel = e.config.Default.SummaryModel
		}
		summaryPrompt = e.config.Default.SummaryPrompt
		embedModel = e.config.Default.ToolCallResult.EmbeddingModel
		if strings.TrimSpace(embedModel) == "" {
			embedModel = e.config.Default.Embedder
		}
		defaultModel = e.config.Default.Model
	}
	gtool.AddInternalService(e.tools, msgsvc.NewWithDeps(e.convClient, e.llmCore, e.embedderFinder, summarizeChunk, matchChunk, summaryModel, summaryPrompt, defaultModel, embedModel))

	// Apply per-model tool result preview limits from model configs when available
	if e.llmCore != nil && e.config != nil && e.config.Model != nil {
		limits := map[string]int{}
		for _, cfg := range e.config.Model.Items {
			if cfg == nil {
				continue
			}
			if cfg.Options.ToolResultPreviewLimit > 0 {
				limits[cfg.ID] = cfg.Options.ToolResultPreviewLimit
			}
		}
		if len(limits) > 0 {
			e.llmCore.SetModelPreviewLimits(limits)
		}
	}
	convHandler := func(ctx context.Context, in *agent2.QueryInput, out *agent2.QueryOutput) error {
		exec, err := agentSvc.Method("query")
		if err != nil {
			return err
		}
		return exec(ctx, in, out)
	}
	e.convManager = conversation.New(convHandler)

	return nil
}

// A2AAuthProxy mirrors agent.A2AAuth to avoid an import cycle in bootstrap.
type A2AAuthProxy struct {
	Enabled       bool
	Resource      string
	Scopes        []string
	UseIDToken    bool
	ExcludePrefix string
}

// validateExtensions loads all feed specs from the workspace and fails when any
// definition cannot be parsed. This enforces early, explicit failures instead of
// silent skips later during request handling.
func (e *Service) validateExtensions(ctx context.Context) error {
	repo := extrepo.New(afs.New())
	names, err := repo.List(ctx)
	if err != nil {
		return fmt.Errorf("feeds: list failed: %w", err)
	}
	var bad []string
	for _, n := range names {
		if _, err := repo.Load(ctx, n); err != nil {
			bad = append(bad, fmt.Sprintf("%s: %v", n, err))
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("feeds: invalid definitions: %s", strings.Join(bad, "; "))
	}
	return nil
}

// initDefaults sets fall-back implementations for all dependencies that were
// not provided through options.
func (e *Service) initDefaults(ctx context.Context) {
	if e.config == nil {
		e.config = &Config{}
	}

	// Load default workspace config.yaml when no explicit config was supplied.
	// This makes CLI/HTTP entry-points that construct executor.Service directly
	// respect $AGENTLY_ROOT/ag/config.yaml without going through instance.Init.
	e.loadWorkspaceConfigIfEmpty(ctx)
	// Ensure toolCallResult defaults when missing
	if e.config != nil {
		tr := &e.config.Default.ToolCallResult
		if tr.PreviewLimit == 0 {
			tr.PreviewLimit = 8192
		}
		if tr.SummarizeChunk == 0 {
			tr.SummarizeChunk = 4096
		}
		if tr.MatchChunk == 0 {
			tr.MatchChunk = 1024
		}
		// Prefer explicit summary model; otherwise default to the global default model id
		if strings.TrimSpace(tr.SummaryModel) == "" {
			tr.SummaryModel = strings.TrimSpace(e.config.Default.Model)
		}
		if strings.TrimSpace(tr.EmbeddingModel) == "" {
			tr.EmbeddingModel = e.config.Default.Embedder
		}
		if e.config.A2ATimeoutSec <= 0 {
			e.config.A2ATimeoutSec = 120
		}
	}
	e.initModel()
	e.initEmbedders()
	e.initAgent(ctx)
	e.initMcp()

	// Default approval service (in-memory) when not injected
	if e.approvalSvc == nil {
		e.approvalSvc = apprmem.New()
	}

	if e.modelFinder == nil {
		finder := e.config.DefaultModelFinder()
		e.modelFinder = finder
		e.modelMatcher = finder.Matcher()
	}
	if e.embedderFinder == nil {
		e.embedderFinder = e.config.DefaultEmbedderFinder()
	}

}

// loadWorkspaceConfigIfEmpty attempts to load $AGENTLY_ROOT/config.yaml (or the
// Config.BaseURL root) into e.config when the current config appears empty.
func (e *Service) loadWorkspaceConfigIfEmpty(ctx context.Context) {
	// consider config empty when all groups are nil and no base/dao/services set
	isEmpty := func(c *Config) bool {
		if c == nil {
			return true
		}
		if strings.TrimSpace(c.BaseURL) != "" { // has explicit base
			return false
		}
		if c.Agent != nil || c.Model != nil || c.Embedder != nil || c.MCP != nil || c.DAOConnector != nil {
			return false
		}
		if len(c.Services) > 0 {
			return false
		}
		// Defaults may be zero-value; we don't try to introspect deeply
		return true
	}
	if !isEmpty(e.config) {
		return
	}

	base := e.config.BaseURL
	if strings.TrimSpace(base) == "" {
		base = workspace.Root()
	}
	cfgPath := filepath.Join(base, "config.yaml")
	fs := afs.New()
	if ok, _ := fs.Exists(ctx, cfgPath); !ok {
		return
	}
	data, err := fs.DownloadWithURL(ctx, cfgPath)
	if err != nil {
		return
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return
	}
	// Replace the empty config with loaded one.
	e.config = &cfg
}

func (e *Service) initModel() {
	// merge model repo first so DefaultModelFinder sees them
	if e.config.Model == nil {
		e.config.Model = &mcpcfg.Group[*llmprovider.Config]{}
	}
	repo := modelrepo.New(afs.New())
	if names, err := repo.List(context.Background()); err == nil {
		for _, n := range names {
			cfg, err := repo.Load(context.Background(), n)
			if err != nil || cfg == nil {
				continue
			}
			dup := false
			for _, ex := range e.config.Model.Items {
				if ex != nil && ex.ID == cfg.ID {
					dup = true
					break
				}
			}
			if !dup {
				e.config.Model.Items = append(e.config.Model.Items, cfg)
			}
		}
	}
}

func (e *Service) initEmbedders() {
	// merge model repo first so DefaultModelFinder sees them
	if e.config.Embedder == nil {
		e.config.Embedder = &mcpcfg.Group[*embedderprovider.Config]{}
	}
	repo := embedderrepo.New(afs.New())
	if names, err := repo.List(context.Background()); err == nil {
		for _, n := range names {
			cfg, err := repo.Load(context.Background(), n)
			if err != nil || cfg == nil {
				continue
			}
			dup := false
			for _, ex := range e.config.Embedder.Items {
				if ex != nil && ex.ID == cfg.ID {
					dup = true
					break
				}
			}
			if !dup {
				e.config.Embedder.Items = append(e.config.Embedder.Items, cfg)
			}
		}
	}
}

func (e *Service) initMcp() {
	// Merge MCP repo entries -----------------------------
	if e.config.MCP == nil {
		e.config.MCP = &mcpcfg.Group[*mcpcfg.MCPClient]{}
	}

	if e.clientHandler == nil {
		// Ensure router is not nil for elicitation service
		if e.elicitationRouter == nil {
			e.elicitationRouter = elicrouter.New()
		}
		// Build elicitation service for MCP client; provide router and optional interactive awaiter
		el := elicitation.New(e.convClient, nil, e.elicitationRouter, e.newAwaiter)
		e.clientHandler = clientmcp.NewClient(el, e.convClient, nil)
	}
	repo := mcprepo.New(afs.New())
	if names, err := repo.List(context.Background()); err != nil {
		// Print error and continue without failing executor initialisation.
		log.Printf("mcp: listing servers failed: %v", err)
	} else {
		for _, n := range names {
			opt, err := repo.Load(context.Background(), n)
			if err != nil {
				log.Printf("mcp: load %s failed: %v", n, err)
				continue
			}
			if opt == nil {
				continue
			}
			dup := false
			for _, ex := range e.config.MCP.Items {
				if ex != nil && ex.Name == opt.Name {
					dup = true
					break
				}
			}
			if dup {
				continue
			}
			var clone mcpcfg.MCPClient
			if b, err := yaml.Marshal(opt); err == nil {
				_ = yaml.Unmarshal(b, &clone)
				e.config.MCP.Items = append(e.config.MCP.Items, &clone)
			}
		}
	}

	// Ensure a default MCP manager exists when not injected via options.
	if e.mcpMgr == nil {
		prov := mcpmgr.NewRepoProvider()
		// Reuse the already-initialised client handler so that elicitations and
		// conversation persistence are consistent across CLI/HTTP flows.
		hFactory := func() protoclient.Handler { return e.clientHandler }
		e.mcpMgr = mcpmgr.New(prov, mcpmgr.WithHandlerFactory(hFactory))
	}

}

func (e *Service) initAgent(ctx context.Context) {
	// Merge agent repo into config.Agent.Group if not duplicates by ID
	if e.config.Agent == nil {
		e.config.Agent = &mcpcfg.Group[*agent.Agent]{}
	}
	if e.agentFinder == nil {
		e.agentFinder = e.config.DefaultAgentFinder()
	}

	agentRepo := agentrepo.New(afs.New())
	if names, err := agentRepo.List(context.Background()); err == nil {
		for _, n := range names {
			a, err := e.agentFinder.Find(ctx, n)
			if err != nil {
				log.Printf("agent: failed to load %s: %v", n, err)
				continue
			}
			if a == nil {
				log.Printf("agent: loader returned nil for %s without error; skipping", n)
				continue
			}
			dup := false
			for _, ex := range e.config.Agent.Items {
				if ex != nil && ex.ID == a.ID {
					dup = true
					break
				}
			}
			if !dup {
				e.config.Agent.Items = append(e.config.Agent.Items, a)
			}
		}
	}
}

// loadDAOConfig loads DAO connector config from inline value or from external
// YAML/JSON document referenced by Config.DAOConnectorURL.
func (e *Service) loadDAOConfig(ctx context.Context) (*view.DBConfig, error) {
	if e.config == nil {
		return nil, nil
	}
	if e.config.DAOConnector != nil && strings.TrimSpace(e.config.DAOConnector.Name) != "" {
		return e.config.DAOConnector, nil // legacy inline form
	}
	return nil, nil
}
