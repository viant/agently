package executor

import (
	"context"
	"fmt"
	clientmcp "github.com/viant/agently/adapter/mcp"
	mcpmap "github.com/viant/agently/genai/adapter/mcp"
	convdao "github.com/viant/agently/internal/dao/conversation"
	"gopkg.in/yaml.v3"
	"strings"

	"github.com/viant/afs"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/tool"
	"github.com/viant/datly/view"
	"github.com/viant/fluxor"
	"github.com/viant/mcp"
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
	return e.workflow.Runtime.Start(ctx)
}

// initDefaults sets fall-back implementations for all dependencies that were
// not provided through options.
func (e *Service) initDefaults() {
	if e.config == nil {
		e.config = &Config{}
	}
	if e.mcpClient == nil {
		e.mcpClient = clientmcp.NewClient()
	}
	if e.modelFinder == nil {
		e.modelFinder = e.config.DefaultModelFinder()
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

	e.workflow.Service = fluxor.New(options...)
	e.workflow.Runtime = e.workflow.Service.Runtime()
	actions := e.registerServices()
	// Register user-provided extension services
	for _, svc := range e.workflow.Extensions {
		actions.Register(svc)
	}
}
