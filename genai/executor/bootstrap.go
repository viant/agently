package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/viant/afs"
	clientmcp "github.com/viant/agently/adapter/mcp"
	tooladapter "github.com/viant/agently/adapter/tool"
	mcpmap "github.com/viant/agently/genai/adapter/mcp"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/tool"
	convdao "github.com/viant/agently/internal/dao/conversation"
	elog "github.com/viant/agently/internal/log"
	"github.com/viant/datly/view"
	"github.com/viant/fluxor"
	"github.com/viant/fluxor/model/graph"
	"github.com/viant/fluxor/runtime/execution"
	"github.com/viant/fluxor/service/action/system/exec"
	texecutor "github.com/viant/fluxor/service/executor"
	"github.com/viant/mcp"
	"gopkg.in/yaml.v3"
	"strings"
)

// init prepares the Service for handling requests.
func (e *Service) init(ctx context.Context) error {
	e.initDefaults()
	if err := e.config.Validate(); err != nil {
		return err
	}
	if err := e.initHistory(ctx); err != nil {
		return err
	}
	if err := e.initMcpTools(ctx); err != nil {
		return err
	}
	e.initWorkflowService()

	if anExecutor := e.workflow.Service.Actions().Lookup(exec.Name); anExecutor != nil {
		tooladapter.RegisterActionAsTool(e.workflow.Service, e.tools, exec.Name, "")
	}

	return e.workflow.Runtime.Start(ctx)
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
	if e.tools == nil {
		e.tools = tool.NewRegistry()
	}

	if e.mcpClient == nil {
		e.mcpClient = clientmcp.NewClient(clientmcp.WithLLMCore(e.llmCore))
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

// initMcpTools registers tools exposed by MCP endpoints defined in the config.
func (e *Service) initMcpTools(ctx context.Context) error {
	mcpOptions, err := e.loadMCPOptions(ctx)
	if err != nil {
		return err
	}

	for _, opts := range mcpOptions {
		cli, err := mcp.NewClient(e.mcpClient, opts)
		if err != nil {
			return fmt.Errorf("failed to create mcp client %q: %w", opts.Name, err)
		}

		prefix := opts.Name
		if prefix != "" {
			prefix += "_"
		}

		if err := mcpmap.RegisterTools(ctx, cli, prefix); err != nil {
			return fmt.Errorf("failed to register mcp tools for %q: %w", opts.Name, err)
		}
	}
	return nil
}

// loadMCPOptions loads MCP client options either from inline config or from a
// YAML/JSON document referenced by Config.MCPURL. Inline config takes
// precedence to keep backward compatibility.
func (e *Service) loadMCPOptions(ctx context.Context) ([]*mcp.ClientOptions, error) {
	if e.config.MCP == nil {
		return nil, nil
	}

	if len(e.config.MCP.Items) > 0 { // legacy inline form
		return e.config.MCP.Items, nil
	}

	if e.config.MCP.URL == "" {
		return nil, nil
	}

	fs := afs.New()
	data, err := fs.DownloadWithURL(ctx, e.config.MCP.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to download MCP config %q: %w", e.config.MCP.URL, err)
	}

	var out []*mcp.ClientOptions
	if err := yaml.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("failed to parse MCP config %q: %w", e.config.MCP.URL, err)
	}
	return out, nil
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
func (e *Service) initWorkflowService() {
	options := append(e.workflow.Options,
		fluxor.WithMetaService(e.config.Meta()),
		fluxor.WithRootTaskNodeName("stage"),
		fluxor.WithExtensionTypes(e.workflow.ExtensionTypes...),
	)

	// Debug helper: emit evaluation result of every 'when' expression used by
	// goto/if constructs so that CLI users can spot unexpected truthy values
	// (e.g. len(refinedPlan) > 0 when refinedPlan is empty).
	options = append(options, fluxor.WithWhenListeners(
		func(s *execution.Session, expr string, ok bool) {
			elog.Publish(elog.Event{
				Time:      time.Now(),
				EventType: elog.TaskWhen,
				Payload: map[string]interface{}{
					"expr":   expr,
					"result": ok,
					"state":  s.State,
				},
			})
		},
	))

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
		options = append(options,
			//fluxor.WithStateListeners(func(s *execution.Session, key string, oldVal, newVal interface{}) {}),
			fluxor.WithExecutorOptions(texecutor.WithListener(listener)))

	}
	options = append(options, fluxor.WithExecutorOptions(texecutor.WithApprovalSkipPrefixes("llm/")))

	e.workflow.Service = fluxor.New(options...)
	e.workflow.Runtime = e.workflow.Service.Runtime()
	actions := e.registerServices()
	// Register user-provided extension services
	for _, svc := range e.workflow.Extensions {
		actions.Register(svc)
	}
}
