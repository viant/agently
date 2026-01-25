package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	neturl "net/url"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/viant/afs"
	clientmcp "github.com/viant/agently/adapter/mcp"
	"github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/elicitation"
	elicrouter "github.com/viant/agently/genai/elicitation/router"
	embedderprovider "github.com/viant/agently/genai/embedder/provider"
	llmprovider "github.com/viant/agently/genai/llm/provider"
	gtool "github.com/viant/agently/genai/tool"
	"github.com/viant/agently/internal/workspace"
	fsloader "github.com/viant/agently/internal/workspace/loader/fs"
	modelload "github.com/viant/agently/internal/workspace/loader/model"
	"github.com/viant/agently/internal/workspace/repository/agent"
	embedderrepo "github.com/viant/agently/internal/workspace/repository/embedder"
	extrepo "github.com/viant/agently/internal/workspace/repository/extension"
	mcprepo "github.com/viant/agently/internal/workspace/repository/mcp"
	"github.com/viant/datly/view"
	// decoupled from orchestration
	authctx "github.com/viant/agently/internal/auth"
	integrate "github.com/viant/agently/internal/auth/mcp/integrate"
	mcpcfg "github.com/viant/agently/internal/mcp/config"
	mcpcookies "github.com/viant/agently/internal/mcp/cookies"
	mcpmgr "github.com/viant/agently/internal/mcp/manager"
	mcpuri "github.com/viant/agently/internal/mcp/uri"
	protoclient "github.com/viant/mcp-protocol/client"
	authtransport "github.com/viant/mcp/client/auth/transport"
	// mcp client types not used here
	"gopkg.in/yaml.v3"

	"github.com/viant/agently/genai/conversation"
	"github.com/viant/agently/genai/memory"
	agent2 "github.com/viant/agently/genai/service/agent"
	augmenter "github.com/viant/agently/genai/service/augmenter"
	mcpfs "github.com/viant/agently/genai/service/augmenter/mcpfs"
	core "github.com/viant/agently/genai/service/core"
	llmagents "github.com/viant/agently/genai/tool/service/llm/agents"
	llmexectool "github.com/viant/agently/genai/tool/service/llm/exec"
	msgsvc "github.com/viant/agently/genai/tool/service/message"
	rsrcsvc "github.com/viant/agently/genai/tool/service/resources"
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
	if err := e.initDefaults(ctx); err != nil {
		return err
	}
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
		// Emit debug events from tool registry to standard logger for better MCP visibility
		if dl, ok := interface{}(reg).(interface{ SetDebugLogger(w io.Writer) }); ok {
			dl.SetDebugLogger(log.Writer())
		}
		reg.Initialize(ctx)
		// Surface any initialization warnings (e.g., unreachable MCP servers)
		if w, ok := interface{}(reg).(interface {
			LastWarnings() []string
			ClearWarnings()
		}); ok {
			for _, msg := range w.LastWarnings() {
				log.Printf("[mcp:init] warning: %s", strings.TrimSpace(msg))
			}
			w.ClearWarnings()
		}
		// Expose internal agents as virtual tools driven by Profile.Publish
		if e.config != nil && e.config.Agent != nil && len(e.config.Agent.Items) > 0 {
			gtool.InjectVirtualAgentTools(reg, e.config.Agent.Items, "")
		}
		e.tools = reg
	}

	// Initialise decoupled core/agent services and conversation manager
	var upstreamConcurrency int
	var matchConcurrency int
	indexAsync := true
	if e.config != nil {
		upstreamConcurrency = e.config.Default.Resources.UpstreamSyncConcurrency
		matchConcurrency = e.config.Default.Resources.MatchConcurrency
		if e.config.Default.Resources.IndexAsync != nil {
			indexAsync = *e.config.Default.Resources.IndexAsync
		}
	}
	snapResolver := buildSnapshotResolver(ctx, e.mcpMgr)
	manifestResolver := buildSnapshotManifestResolver(ctx, e.mcpMgr)
	enricher := augmenter.New(
		e.embedderFinder,
		augmenter.WithMCPManager(e.mcpMgr),
		augmenter.WithMCPSnapshotResolver(snapResolver),
		augmenter.WithMCPSnapshotManifestResolver(manifestResolver),
		augmenter.WithUpstreamSyncConcurrency(upstreamConcurrency),
		augmenter.WithMatchConcurrency(matchConcurrency),
		augmenter.WithIndexAsync(indexAsync),
	)
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
	// Register resources service to expose roots/list/match over files and MCP resources
	var rdef rsrcsvc.ResourcesDefaults
	rdef.Locations = nil
	rdef.TrimPath = ""
	rdef.SummaryFiles = nil
	if e.config != nil {
		rdef.Locations = append(rdef.Locations, e.config.Default.Resources.Locations...)
		rdef.TrimPath = e.config.Default.Resources.TrimPath
		if len(e.config.Default.Resources.SummaryFiles) > 0 {
			rdef.SummaryFiles = append(rdef.SummaryFiles, e.config.Default.Resources.SummaryFiles...)
		}
		rdef.DescribeMCP = e.config.Default.Resources.DescribeMCP
	}
	gtool.AddInternalService(e.tools, rsrcsvc.New(
		enricher,
		rsrcsvc.WithMCPManager(e.mcpMgr),
		rsrcsvc.WithDefaults(rdef),
		rsrcsvc.WithConversationClient(e.convClient),
		rsrcsvc.WithAgentFinder(e.agentFinder),
		rsrcsvc.WithDefaultEmbedder(e.config.Default.Embedder),
	))
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
			// llm/agents:* directory and routing should include all configured agents.
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
				id := strings.TrimSpace(a.ID)
				name := ""
				if a.Profile != nil {
					name = strings.TrimSpace(a.Profile.Name)
				}
				if name == "" {
					name = strings.TrimSpace(a.Name)
				}
				if name == "" {
					name = id
				}
				desc := ""
				if a.Profile != nil {
					desc = strings.TrimSpace(a.Profile.Description)
				}
				if desc == "" {
					desc = strings.TrimSpace(a.Description)
				}
				var caps map[string]interface{}
				if a.Profile != nil && a.Profile.Capabilities != nil {
					caps = a.Profile.Capabilities
				}
				// Optional responsibilities and scope info for better orchestration context
				var resp, inScope, outScope []string
				if a.Profile != nil {
					if len(a.Profile.Responsibilities) > 0 {
						resp = append([]string(nil), a.Profile.Responsibilities...)
					}
					if len(a.Profile.InScope) > 0 {
						inScope = append([]string(nil), a.Profile.InScope...)
					}
					if len(a.Profile.OutOfScope) > 0 {
						outScope = append([]string(nil), a.Profile.OutOfScope...)
					}
				}
				var tags []string
				if a.Profile != nil && len(a.Profile.Tags) > 0 {
					tags = append([]string(nil), a.Profile.Tags...)
				}
				rank := 0
				if a.Profile != nil {
					rank = a.Profile.Rank
				}
				items = append(items, llmagents.ListItem{
					ID:               id,
					Name:             name,
					Description:      desc,
					Tags:             tags,
					Priority:         rank,
					Capabilities:     caps,
					Source:           "internal",
					Responsibilities: resp,
					InScope:          inScope,
					OutOfScope:       outScope,
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
		if e.config.Default.PreviewSettings.SummarizeChunk > 0 {
			summarizeChunk = e.config.Default.PreviewSettings.SummarizeChunk
		}
		if e.config.Default.PreviewSettings.MatchChunk > 0 {
			matchChunk = e.config.Default.PreviewSettings.MatchChunk
		}
		summaryModel = e.config.Default.PreviewSettings.SummaryModel
		if strings.TrimSpace(summaryModel) == "" {
			summaryModel = e.config.Default.SummaryModel
		}
		summaryPrompt = e.config.Default.SummaryPrompt
		embedModel = e.config.Default.PreviewSettings.EmbeddingModel
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
func (e *Service) initDefaults(ctx context.Context) error {
	if e.config == nil {
		e.config = &Config{}
	}

	// Load default workspace config.yaml when no explicit config was supplied.
	// This makes CLI/HTTP entry-points that construct executor.Service directly
	// respect $AGENTLY_WORKSPACE/ag/config.yaml without going through instance.Init.
	e.loadWorkspaceConfigIfEmpty(ctx)
	// Ensure toolCallResult defaults when missing
	if e.config != nil {

		defaults := &e.config.Default
		if defaults.ToolCallMaxResults == 0 {
			defaults.ToolCallMaxResults = 100
		}
		// Set a sensible default tool-call timeout when not provided (5 minutes)
		if defaults.ToolCallTimeoutSec <= 0 {
			defaults.ToolCallTimeoutSec = 300
		}
		tr := &e.config.Default.PreviewSettings

		// the real max limit is about 0.9 * tr.Limit (see buildOverflowPreview func)
		if tr.Limit == 0 {
			tr.Limit = 16384
		}
		if tr.AgedAfterSteps == 0 {
			tr.AgedAfterSteps = 80
		}
		if tr.AgedAfterSteps == 0 {
			tr.AgedLimit = 2048
		}
		if tr.SummarizeChunk == 0 {
			tr.SummarizeChunk = 4096
		}
		if tr.MatchChunk == 0 {
			tr.MatchChunk = 1024
		}
		if tr.SummaryThresholdBytes <= 0 {
			// Default: enable summarize helpers only for messages
			// larger than 128KB. Smaller overflows can still use
			// internal/message:show and, when enabled, match.
			tr.SummaryThresholdBytes = 256 * 1024
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
	err := e.initModel()
	if err != nil {
		return err
	}

	e.initEmbedders()
	e.initAgent(ctx)
	if err := e.initMcp(ctx); err != nil {
		return err
	}

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

	return nil
}

// loadWorkspaceConfigIfEmpty attempts to load $AGENTLY_WORKSPACE/config.yaml (or the
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

func (e *Service) initModel() error {
	// Merge workspace model configs first so DefaultModelFinder sees them.
	// Use the loader/decoder path to parse provider options consistently.
	if e.config.Model == nil {
		e.config.Model = &mcpcfg.Group[*llmprovider.Config]{}
	}

	loader := modelload.New(fsloader.WithMetaService[llmprovider.Config](e.config.Meta()))
	cfgs, err := loader.List(context.Background(), workspace.KindModel)
	if err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(e.config.Model.Items))
	for _, ex := range e.config.Model.Items {
		if ex == nil {
			continue
		}
		seen[ex.ID] = struct{}{}
	}

	for _, cfg := range cfgs {
		if cfg == nil {
			continue
		}
		if _, exists := seen[cfg.ID]; exists {
			continue
		}
		e.config.Model.Items = append(e.config.Model.Items, cfg)
		seen[cfg.ID] = struct{}{}
	}

	return nil

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

func (e *Service) initMcp(ctx context.Context) error {
	// Ensure MCP group present
	if err := e.ensureMCPGroup(); err != nil {
		return err
	}
	// Ensure client handler (elicitation-backed) present
	e.ensureClientHandler()

	// Merge MCP repo entries
	if err := e.mergeMCPRepoEntries(ctx); err != nil {
		return err
	}

	// Ensure default manager
	if err := e.ensureMCPManager(); err != nil {
		return err
	}
	return nil
}

// ensureMCPGroup guarantees the MCP group in config exists.
func (e *Service) ensureMCPGroup() error {
	if e.config == nil {
		return fmt.Errorf("executor: config is nil")
	}
	if e.config.MCP == nil {
		e.config.MCP = &mcpcfg.Group[*mcpcfg.MCPClient]{}
	}
	return nil
}

// ensureClientHandler initialises the MCP client handler with elicitation support when missing.
func (e *Service) ensureClientHandler() {
	if e.clientHandler != nil {
		return
	}
	if e.elicitationRouter == nil {
		e.elicitationRouter = elicrouter.New()
	}
	el := elicitation.New(e.convClient, nil, e.elicitationRouter, e.newAwaiter)
	e.clientHandler = clientmcp.NewClient(el, e.convClient, nil)
}

// mergeMCPRepoEntries loads MCP client configs from the workspace repository and merges
// them into the service config, skipping duplicates by Name. Collects all errors.
func (e *Service) mergeMCPRepoEntries(ctx context.Context) error {
	if e == nil || e.config == nil {
		return fmt.Errorf("mcp: config not initialized")
	}
	if e.config.MCP == nil {
		e.config.MCP = &mcpcfg.Group[*mcpcfg.MCPClient]{}
	}
	repo := mcprepo.New(afs.New())
	names, err := repo.List(ctx)
	if err != nil {
		return fmt.Errorf("mcp: list failed: %w", err)
	}
	var errs []string
	for _, n := range names {
		opt, lerr := repo.Load(ctx, n)
		if lerr != nil {
			errs = append(errs, fmt.Sprintf("load %s: %v", n, lerr))
			continue
		}
		if opt == nil {
			continue
		}
		if opt.ClientOptions == nil || strings.TrimSpace(opt.Name) == "" {
			continue
		}
		if e.hasMCPClientByName(opt.Name) {
			continue
		}
		var clone mcpcfg.MCPClient
		b, merr := yaml.Marshal(opt)
		if merr != nil {
			errs = append(errs, fmt.Sprintf("marshal %s: %v", n, merr))
			continue
		}
		if uerr := yaml.Unmarshal(b, &clone); uerr != nil {
			errs = append(errs, fmt.Sprintf("unmarshal %s: %v", n, uerr))
			continue
		}
		e.config.MCP.Items = append(e.config.MCP.Items, &clone)
	}
	if len(errs) > 0 {
		return fmt.Errorf("mcp: merge errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

func (e *Service) hasMCPClientByName(name string) bool {
	if e == nil || e.config == nil || e.config.MCP == nil {
		return false
	}
	for _, ex := range e.config.MCP.Items {
		if ex == nil || ex.ClientOptions == nil {
			continue
		}
		if ex.Name == name {
			return true
		}
	}
	return false
}

// ensureMCPManager initialises the MCP manager with cookie jar and auth RT providers.
func (e *Service) ensureMCPManager() error {
	if e.mcpMgr != nil {
		return nil
	}
	prov := mcpmgr.NewRepoProvider()
	hFactory := func() protoclient.Handler { return e.clientHandler }

	jarProvider := e.newCookieJarProvider()
	authRTProvider := e.newAuthRoundTripperProvider(jarProvider)

	mgr, err := mcpmgr.New(prov,
		mcpmgr.WithHandlerFactory(hFactory),
		mcpmgr.WithCookieJarProvider(jarProvider),
		mcpmgr.WithAuthRoundTripperProvider(authRTProvider),
	)
	if err != nil {
		return fmt.Errorf("mcp manager init: %w", err)
	}
	e.mcpMgr = mgr
	return nil
}

// newCookieJarProvider creates a provider that returns a per-user shared cookie jar
// preloaded from shared and provider-specific jars.
func (e *Service) newCookieJarProvider() func(ctx context.Context) (http.CookieJar, error) {
	fs := afs.New()
	repo := mcprepo.New(fs)
	provider := mcpcookies.New(fs, repo)
	return provider.Jar
}

func (e *Service) newAuthRoundTripperProvider(jarProvider func(ctx context.Context) (http.CookieJar, error)) func(ctx context.Context) *authtransport.RoundTripper {
	var (
		rtMu     sync.Mutex
		rtByUser = map[string]*authtransport.RoundTripper{}
	)
	var prompt integrate.OAuthPrompt
	return func(ctx context.Context) *authtransport.RoundTripper {
		user := strings.TrimSpace(authctx.EffectiveUserID(ctx))
		if user == "" {
			user = "anonymous"
		}
		rtMu.Lock()
		defer rtMu.Unlock()
		if v, ok := rtByUser[user]; ok && v != nil {
			return v
		}
		j, _ := jarProvider(ctx)
		var base http.RoundTripper = http.DefaultTransport
		rt, _ := integrate.NewAuthRoundTripperWithPrompt(j, base, 0, prompt)
		rtByUser[user] = rt
		return rt
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
		if len(names) == 0 {
			// Emit a single helpful warning when no agents are discovered.
			// Expected layouts:
			//   - $AGENTLY_WORKSPACE/agents/<name>.yaml  (preferred)
			//   - $AGENTLY_WORKSPACE/agents/<name>/<name>.yaml  (legacy)
			log.Printf("[workspace] no agents found under %s (root=%s). Ensure agent files exist as <name>.yaml or <name>/<name>.yaml and have directory.enabled=true if they should be listed.", workspace.Path(workspace.KindAgent), workspace.Root())
		}
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

func buildSnapshotResolver(ctx context.Context, mgr *mcpmgr.Manager) mcpfs.SnapshotResolver {
	return func(location string) (snapshotURI, rootURI string, ok bool) {
		if mgr == nil {
			return "", "", false
		}
		server, _ := mcpuri.Parse(location)
		if strings.TrimSpace(server) == "" {
			return "", "", false
		}
		opts, err := mgr.Options(ctx, server)
		if err != nil || opts == nil {
			return "", "", false
		}
		roots := mcpcfg.ResourceRoots(opts.Metadata)
		if len(roots) == 0 {
			return "", "", false
		}
		normLoc := strings.TrimRight(strings.TrimSpace(location), "/")
		if mcpuri.Is(normLoc) {
			normLoc = mcpuri.NormalizeForCompare(normLoc)
		}
		for _, root := range roots {
			if !root.Snapshot {
				continue
			}
			uri := strings.TrimRight(strings.TrimSpace(root.URI), "/")
			if uri == "" {
				continue
			}
			if mcpuri.Is(uri) {
				uri = mcpuri.NormalizeForCompare(uri)
			}
			if normLoc == uri || strings.HasPrefix(normLoc, uri+"/") {
				rootURI = uri
				snapshotURI = strings.TrimSpace(root.SnapshotURI)
				if snapshotURI == "" {
					snapshotURI = rootURI + "/_snapshot.zip"
				}
				return snapshotURI, rootURI, true
			}
		}
		return "", "", false
	}
}

func buildSnapshotManifestResolver(ctx context.Context, mgr *mcpmgr.Manager) mcpfs.SnapshotManifestResolver {
	return func(location string) bool {
		if mgr == nil {
			return false
		}
		server, _ := mcpuri.Parse(location)
		if strings.TrimSpace(server) == "" {
			return false
		}
		opts, err := mgr.Options(ctx, server)
		if err != nil || opts == nil {
			return false
		}
		roots := mcpcfg.ResourceRoots(opts.Metadata)
		if len(roots) == 0 {
			return false
		}
		normLoc := strings.TrimRight(strings.TrimSpace(location), "/")
		if mcpuri.Is(normLoc) {
			normLoc = mcpuri.NormalizeForCompare(normLoc)
		}
		for _, root := range roots {
			if !root.Snapshot || !root.SnapshotMD5 {
				continue
			}
			uri := strings.TrimRight(strings.TrimSpace(root.URI), "/")
			if uri == "" {
				continue
			}
			if mcpuri.Is(uri) {
				uri = mcpuri.NormalizeForCompare(uri)
			}
			if normLoc == uri || strings.HasPrefix(normLoc, uri+"/") {
				return true
			}
		}
		return false
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
