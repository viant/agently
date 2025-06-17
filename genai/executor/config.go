package executor

import (
	"github.com/viant/afs"
	"github.com/viant/agently/genai/agent"
	embedderprovider "github.com/viant/agently/genai/embedder/provider"
	agentfinder "github.com/viant/agently/internal/finder/agent"
	embedderfinder "github.com/viant/agently/internal/finder/embedder"
	agentloader "github.com/viant/agently/internal/loader/agent"
	embedderlloader "github.com/viant/agently/internal/loader/embedder"
	"github.com/viant/agently/internal/workspace"
	"github.com/viant/fluxor/service/meta"

	llmprovider "github.com/viant/agently/genai/llm/provider"
	modelfinder "github.com/viant/agently/internal/finder/model"
	modelloader "github.com/viant/agently/internal/loader/model"

	"github.com/viant/agently/internal/loader/fs"
	"github.com/viant/datly/view"
	mcpcfg "github.com/viant/fluxor-mcp/mcp/config"
	"github.com/viant/mcp"
)

type Default struct {
	Model string
	Agent string
}

type Config struct {
	BaseURL      string                                  `yaml:"baseUrl" json:"baseUrl" `
	Agent        *mcpcfg.Group[*agent.Agent]             `yaml:"agents" json:"agents"`
	Model        *mcpcfg.Group[*llmprovider.Config]      `yaml:"models" json:"models"`
	Embedder     *mcpcfg.Group[*embedderprovider.Config] `yaml:"embedders" json:"embedders" `
	MCP          *mcpcfg.Group[*mcp.ClientOptions]       `yaml:"mcp" json:"mcp"`
	DAOConnector *view.DBConfig                          `yaml:"daoConfig" json:"daoConfig" `
	Default      Default                                 `yaml:"default" json:"default"`

	ToolRetries int
	//
	metaService *meta.Service
	Services    []string `yaml:"services" json:"services"`
}

func (c *Config) Meta() *meta.Service {
	if c.metaService != nil {
		return c.metaService
	}
	baseURL := c.BaseURL
	if baseURL == "" {
		// Default to the centralised workspace root when caller did not
		// specify a BaseURL explicitly.
		baseURL = workspace.Root()
	}
	c.metaService = meta.New(afs.New(), baseURL)
	return c.metaService
}

func (c *Config) DefaultModelFinder() *modelfinder.Finder {
	var options = []fs.Option[llmprovider.Config]{
		fs.WithMetaService[llmprovider.Config](c.Meta()),
	}
	return modelfinder.New(
		modelfinder.WithInitial(c.Model.Items...),
		modelfinder.WithConfigLoader(modelloader.New(options...)),
	)
}

func (c *Config) DefaultEmbedderFinder() *embedderfinder.Finder {
	if c.Embedder == nil {
		return embedderfinder.New()
	}
	var options = []fs.Option[embedderprovider.Config]{
		fs.WithMetaService[embedderprovider.Config](c.Meta()),
	}
	return embedderfinder.New(embedderfinder.WithInitial(c.Embedder.Items...),
		embedderfinder.WithConfigLoader(embedderlloader.New(options...)),
	)
}

func (c *Config) DefaultAgentFinder(options ...agentloader.Option) *agentfinder.Finder {
	// Always resolve relative agent paths against the workspace root (or
	// Config.BaseURL when explicitly set) so that callers can simply refer to
	// "chat" instead of providing an absolute path.

	// Append meta service option to ensure agent loader uses the same baseURL
	// strategy as other component loaders.  Intentionally append (not prepend)
	// so that explicit caller-supplied WithMetaService overrides ours when
	// needed.
	options = append(options, agentloader.WithMetaService(c.Meta()))

	return agentfinder.New(
		agentfinder.WithInitial(c.Agent.Items...),
		agentfinder.WithLoader(agentloader.New(options...)),
	)
}

func (c *Config) Validate() error {
	return nil
}
