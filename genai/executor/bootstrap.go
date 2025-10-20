package executor

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"

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
	agent2 "github.com/viant/agently/genai/service/agent"
	augmenter "github.com/viant/agently/genai/service/augmenter"
	core "github.com/viant/agently/genai/service/core"
	llmexectool "github.com/viant/agently/genai/tool/service/llm/exec"
	msgsvc "github.com/viant/agently/genai/tool/service/message"
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
		// Expose agents as virtual tools when supported
		gtool.InjectVirtualAgentTools(reg, e.config.Agent.Items, "")
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

	// Register llm/exec service as internal MCP for run_agent using Service.Name().
	gtool.AddInternalService(e.tools, llmexectool.New(agentSvc))
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
			if err != nil || a == nil {
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
