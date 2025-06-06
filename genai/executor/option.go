package executor

import (
	"github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/io/elicitation"
	modelprovider "github.com/viant/agently/genai/llm/provider"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/tool"
	"github.com/viant/fluxor"
	mcpcfg "github.com/viant/fluxor-mcp/mcp/config"
	"github.com/viant/fluxor/service/meta"
	"io"
)

type Option func(config *Service)

// WithElicitationAwaiter registers an interactive Awaiter responsible for
// resolving schema-driven user prompts ("elicitation"). When set, the awaiter
// is attached to the internally managed MCP client so that interactive CLI
// sessions can synchronously obtain the required payload.
//
// The last non-nil value wins when the option is applied multiple times.
func WithElicitationAwaiter(a elicitation.Awaiter) Option {
	return func(s *Service) {
		if a != nil {
			s.MCPElicitationAwaiter = a
		}
	}
}

func WithConfig(config *Config) Option {
	return func(s *Service) {
		s.config = config
	}
}

// WithLLMLogger redirects all LLM prompt/response traffic captured by the core
// LLM service to the supplied writer. Passing nil disables logging.
func WithLLMLogger(w io.Writer) Option {
	return func(s *Service) {
		s.llmLogger = w
	}
}

// WithFluxorActivityLogger writes every executed Fluxor task (as seen by the
// executor listener) to the supplied writer in JSON. Passing nil disables
// logging.
func WithFluxorActivityLogger(w io.Writer) Option {
	return func(s *Service) {
		s.fluxorLogWriter = w
	}
}

// WithToolDebugLogger enables debug logging for every tool call executed via
// the executor's tool registry. Each invocation is written to the supplied
// writer. Passing nil disables logging.
func WithToolDebugLogger(w io.Writer) Option {
	return func(s *Service) {
		if s.tools == nil {
			s.tools = tool.NewRegistry()
		}
		s.tools.SetDebugLogger(w)
	}
}

// WithMetaService sets metaService service
func WithMetaService(metaService *meta.Service) Option {
	return func(s *Service) {
		if s.config == nil {
			s.config = &Config{}
		}
		s.config.metaService = metaService
	}
}

// WithWorkflowOptions sets fluxor options
func WithWorkflowOptions(option ...fluxor.Option) Option {
	return func(s *Service) {
		s.fluxorOptions = append(s.fluxorOptions, option...)
	}
}

func WithModelConfig(providers ...*modelprovider.Config) Option {
	return func(s *Service) {
		if s.config.Model == nil {
			s.config.Model = &mcpcfg.Group[*modelprovider.Config]{}
		}
		s.config.Model.Items = append(s.config.Model.Items, providers...)
	}

}

// WithAgents sets a custom agent registry.
func WithAgents(agents ...*agent.Agent) Option {
	return func(s *Service) {
		if s.config.Agent == nil {
			s.config.Agent = &mcpcfg.Group[*agent.Agent]{}
		}
		s.config.Agent.Items = append(s.config.Agent.Items, agents...)
	}
}

// WithTools sets a custom tool registry.
func WithTools(tools *tool.Registry) Option {
	return func(s *Service) {
		s.tools = tools
	}
}

// WithHistory injects a custom conversation history implementation.
func WithHistory(store memory.History) Option {
	return func(s *Service) {
		s.history = store
	}
}

// WithToolRetries sets the default retry count for tool execution steps.
func WithToolRetries(maxRetries int) Option {
	return func(s *Service) {
		s.config.ToolRetries = maxRetries
	}
}
