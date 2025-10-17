package executor

import (
	"github.com/viant/afs"
	"github.com/viant/agently/genai/agent"
	embedderprovider "github.com/viant/agently/genai/embedder/provider"
	"github.com/viant/agently/genai/executor/config"
	llmprovider "github.com/viant/agently/genai/llm/provider"
	authcfg "github.com/viant/agently/internal/auth"
	agentfinder "github.com/viant/agently/internal/finder/agent"
	embedderfinder "github.com/viant/agently/internal/finder/embedder"
	modelfinder "github.com/viant/agently/internal/finder/model"
	mcpcfg "github.com/viant/agently/internal/mcp/config"
	"github.com/viant/agently/internal/workspace"
	agent2 "github.com/viant/agently/internal/workspace/loader/agent"
	embedderlloader "github.com/viant/agently/internal/workspace/loader/embedder"
	"github.com/viant/agently/internal/workspace/loader/fs"
	modelloader "github.com/viant/agently/internal/workspace/loader/model"
	meta "github.com/viant/agently/internal/workspace/service/meta"

	"github.com/viant/datly/view"
)

type Config struct {
	BaseURL      string                                  `yaml:"baseUrl" json:"baseUrl" `
	Agent        *mcpcfg.Group[*agent.Agent]             `yaml:"agents" json:"agents"`
	Model        *mcpcfg.Group[*llmprovider.Config]      `yaml:"models" json:"models"`
	Embedder     *mcpcfg.Group[*embedderprovider.Config] `yaml:"embedders" json:"embedders" `
	MCP          *mcpcfg.Group[*mcpcfg.MCPClient]        `yaml:"mcp" json:"mcp"`
	DAOConnector *view.DBConfig                          `yaml:"daoConfig" json:"daoConfig" `
	Default      config.Defaults                         `yaml:"default" json:"default"`
	Auth         *authcfg.Config                         `yaml:"auth" json:"auth"`
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
		// Defaults to the centralised workspace root when caller did not
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

func (c *Config) DefaultAgentFinder(options ...agent2.Option) *agentfinder.Finder {
	// Always resolve relative agent paths against the workspace root (or
	// Config.BaseURL when explicitly set) so that callers can simply refer to
	// "chat" instead of providing an absolute path.

	// Append meta service option to ensure agent loader uses the same baseURL
	// strategy as other component loaders.  Intentionally append (not prepend)
	// so that explicit caller-supplied WithMetaService overrides ours when
	// needed.
	options = append(options, agent2.WithMetaService(c.Meta()))

	return agentfinder.New(
		agentfinder.WithInitial(c.Agent.Items...),
		agentfinder.WithLoader(agent2.New(options...)),
	)
}

func (c *Config) Validate() error {
	return nil
}
