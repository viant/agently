package executor

import (
	"context"
	"encoding/json"
	"fmt"
	clientmcp "github.com/viant/agently/adapter/mcp"
	"github.com/viant/agently/adapter/tool"
	"github.com/viant/agently/genai/memory"
	convdao "github.com/viant/agently/internal/dao/conversation"

	"github.com/viant/datly/view"
	"github.com/viant/fluxor"
	mcpsvc "github.com/viant/fluxor-mcp/mcp"
	mcpcfg "github.com/viant/fluxor-mcp/mcp/config"
	"github.com/viant/fluxor/model/graph"
	"github.com/viant/fluxor/runtime/execution"

	texecutor "github.com/viant/fluxor/service/executor"
	"strings"
)

// init prepares the Service for handling requests.
func (e *Service) init(ctx context.Context) error {
	// ------------------------------------------------------------------
	// Step 1: defaults & validation
	// ------------------------------------------------------------------
	e.initDefaults()
	if err := e.config.Validate(); err != nil {
		return err
	}

	// ------------------------------------------------------------------
	// Step 2: auxiliary stores (history, …)
	// ------------------------------------------------------------------
	if err := e.initHistory(ctx); err != nil {
		return err
	}

	// ------------------------------------------------------------------
	// Step 3: orchestration service (single source of truth for workflows & tools)
	// ------------------------------------------------------------------

	// Translate executor Config → fluxor-mcp Config (reuse MCP section directly)
	mcpConfig := &mcpcfg.Config{
		Builtins: []string{"system/exec"}, // load all auto-discovered built-ins by default
		MCP:      e.config.MCP,
	}

	// Collect additional fluxor options that Agently requires.
	wfOptions := append(e.fluxorOptions,
		fluxor.WithMetaService(e.config.Meta()),
		fluxor.WithRootTaskNodeName("stage"))
	// ------------------------------------------------------------------
	// Debug hooks and executor listener
	// ------------------------------------------------------------------

	if e.fluxorLogWriter != nil {
		listener := func(task *graph.Task, exec *execution.Execution) {
			if task == nil {
				return
			}
			entry := map[string]interface{}{
				"task":   task,
				"input":  exec.Input,
				"output": exec.Output,
				"error":  exec.Error,
			}
			if data, err := json.Marshal(entry); err == nil {
				_, _ = e.fluxorLogWriter.Write(append(data, '\n'))
			}
		}

		wfOptions = append(wfOptions, fluxor.WithExecutorOptions(
			texecutor.WithListener(listener),
			texecutor.WithApprovalSkipPrefixes("llm/"),
		))
	} else {
		wfOptions = append(wfOptions, fluxor.WithExecutorOptions(
			texecutor.WithApprovalSkipPrefixes("llm/"),
		))
	}

	// Build orchestration (fluxor-mcp) service instance
	orchestration, err := mcpsvc.New(ctx,
		mcpsvc.WithConfig(mcpConfig),
		mcpsvc.WithWorkflowOptions(wfOptions...),
		mcpsvc.WithClientHandler(e.clientHandler),
	)
	if err != nil {
		return fmt.Errorf("init orchestration: %w", err)
	}
	e.orchestration = orchestration
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
func (e *Service) initDefaults() {
	if e.config == nil {
		e.config = &Config{}
	}
	if e.modelFinder == nil {
		finder := e.config.DefaultModelFinder()
		e.modelFinder = finder
		e.modelMatcher = finder.Matcher()
		if agent := e.config.Agent; agent != nil && len(agent.Items) > 0 && e.config.Default.Model == "" {
			e.config.Default.Model = agent.Items[0].Model // use first agent's model as default
		}
	}
	if e.embedderFinder == nil {
		e.embedderFinder = e.config.DefaultEmbedderFinder()
	}
	if e.agentFinder == nil {
		e.agentFinder = e.config.DefaultAgentFinder()
	}

	if e.clientHandler == nil {
		var opts []clientmcp.Option
		opts = append(opts, clientmcp.WithLLMCore(e.llmCore))
		if e.MCPElicitationAwaiter != nil {
			opts = append(opts, clientmcp.WithAwaiter(e.MCPElicitationAwaiter))
		}
		e.clientHandler = clientmcp.NewClient(opts...)
	}

}

// initHistory initialises a conversation history store. When a DAO connector
// is configured, a DB-backed implementation is used, otherwise an in-memory
// store is created.
func (e *Service) initHistory(ctx context.Context) error {
	if e.history != nil {
		return nil
	}

	daoCfg, err := e.loadDAOConfig(ctx)
	if err != nil {
		return err
	}

	if daoCfg != nil {
		connector := view.NewConnector(daoCfg.Name, daoCfg.Driver, daoCfg.DSN)
		daoSvc, err := convdao.New(ctx, connector)
		if err != nil {
			return fmt.Errorf("failed to initialise conversation DAO: %w", err)
		}
		e.history = daoSvc
		return nil
	}

	// fall back to in-memory implementation
	e.history = memory.NewHistoryStore()
	return nil
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

// initWorkflowService builds a fluxor Service instance and registers all core
// and extension actions.
// (legacy initWorkflowService removed – runtime is now built by fluxor-mcp)
