package executor

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	clientmcp "github.com/viant/agently/adapter/mcp"
	"github.com/viant/agently/adapter/tool"
	"github.com/viant/agently/genai/agent"
	embedderprovider "github.com/viant/agently/genai/embedder/provider"
	llmprovider "github.com/viant/agently/genai/llm/provider"
	domainrecorder "github.com/viant/agently/internal/domain/recorder"
	agentrepo "github.com/viant/agently/internal/repository/agent"
	embedderrepo "github.com/viant/agently/internal/repository/embedder"
	modelrepo "github.com/viant/agently/internal/repository/model"

	"github.com/viant/afs"
	mcprepo "github.com/viant/agently/internal/repository/mcp"
	"github.com/viant/agently/internal/workspace"
	"github.com/viant/datly/view"
	"github.com/viant/fluxor"
	mcpsvc "github.com/viant/fluxor-mcp/mcp"
	mcpcfg "github.com/viant/fluxor-mcp/mcp/config"
	"github.com/viant/mcp"
	"gopkg.in/yaml.v3"

	texecutor "github.com/viant/fluxor/service/executor"
	// Helpers for exposing agents as Fluxor services
	//	"github.com/viant/agently/genai/executor/agenttool"
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

	// Build recorder (shadow writes when enabled)
	e.recorder = domainrecorder.New(ctx)

	// ------------------------------------------------------------------
	// Step 3: orchestration service (single source of truth for workflows & tools)
	// ------------------------------------------------------------------

	// Translate executor Config → fluxor-mcp Config (reuse MCP section directly)
	mcpConfig := &mcpcfg.Config{
		Builtins: e.config.Services,
		MCP:      e.config.MCP,
	}

	// Collect additional fluxor options that Agently requires.
	wfOptions := append(e.fluxorOptions,
		fluxor.WithMetaService(e.config.Meta()),
		fluxor.WithExecutorOptions(
			texecutor.WithApprovalSkipPrefixes("llm/"),
		),
		fluxor.WithRootTaskNodeName("stage"))

	// Build orchestration (fluxor-mcp) service instance
	orchestration, err := mcpsvc.New(ctx,
		mcpsvc.WithConfig(mcpConfig),
		mcpsvc.WithWorkflowOptions(wfOptions...),
		mcpsvc.WithMcpErrorHandler(func(config *mcp.ClientOptions, err error) error {
			fmt.Printf("mcp %v initialization error: %v\n", config.Name, err)
			return nil
		}),
		mcpsvc.WithClientHandler(e.clientHandler),
	)
	if err != nil {
		return fmt.Errorf("failed to create orchestration service: %w", err)
	}
	e.orchestration = orchestration

	// ------------------------------------------------------------------
	// Step 3b: expose agents as callable tools via orchestration service
	// ------------------------------------------------------------------
	if err := e.registerAgentTools(); err != nil {
		return fmt.Errorf("failed to register agent tools: %w", err)
	}
	if e.tools == nil {
		e.tools = tool.New(e.orchestration)
	}

	// ------------------------------------------------------------------
	// Step 4: register Agently-specific extension services on the shared runtime
	// ------------------------------------------------------------------
	actions := orchestration.WorkflowService().Actions()
	e.registerServices(actions)

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
	e.initModel()
	e.initEmbedders()
	e.initAgent(ctx)
	e.initMcp()

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
		e.config.MCP = &mcpcfg.Group[*mcp.ClientOptions]{}
	}

	if e.clientHandler == nil {
		var opts []clientmcp.Option
		opts = append(opts, clientmcp.WithLLMCore(e.llmCore))
		if e.newAwaiter != nil {
			opts = append(opts, clientmcp.WithAwaiter(e.newAwaiter))
		}
		e.clientHandler = clientmcp.NewClient(opts...)
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
			var clone mcp.ClientOptions
			if b, err := yaml.Marshal(opt); err == nil {
				_ = yaml.Unmarshal(b, &clone)
				e.config.MCP.Items = append(e.config.MCP.Items, &clone)
			}
		}
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
