package executor

import (
	"io"

	atool "github.com/viant/agently/adapter/tool"
	"github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/io/elicitation"
	modelprovider "github.com/viant/agently/genai/llm/provider"
	"github.com/viant/agently/genai/tool"
	"github.com/viant/fluxor"
	mcpcfg "github.com/viant/fluxor-mcp/mcp/config"
	"github.com/viant/fluxor/service/meta"
)

type Option func(config *Service)

// WithNewElicitationAwaiter registers an interactive Awaiter responsible for
// resolving schema-driven user prompts ("elicitation"). When set, the awaiter
// is attached to the internally managed MCP client so that interactive CLI
// sessions can synchronously obtain the required payload.
//
// The last non-nil value wins when the option is applied multiple times.
// WithNewElicitationAwaiter registers (or overrides) the Awaiter that will be used
// to resolve schema-based elicitation requests originating from the runtime.
//
// Passing a non-nil Awaiter enables interactive (or otherwise custom)
// behaviour.  Passing nil explicitly disables any previously registered
// Awaiter – this is useful for headless deployments such as the embedded HTTP
// server where blocking on stdin must be avoided.
//
// The *last* call wins, therefore a later invocation can override an earlier
// one (including the implicit registration performed by the CLI helpers).
func WithNewElicitationAwaiter(newAwaiter func() elicitation.Awaiter) Option {
	return func(s *Service) {
		// Allow nil to reset an earlier registration so that callers like the
		// HTTP server can ensure the executor never blocks on stdin.
		s.newAwaiter = newAwaiter
	}
}

func WithConfig(config *Config) Option {
	return func(s *Service) {
		s.config = config
	}
}

// WithToolDebugLogger enables debug logging for every atool call executed via
// the executor's atool registry. Each invocation is written to the supplied
// writer. Passing nil disables logging.
func WithToolDebugLogger(w io.Writer) Option {
	return func(s *Service) {
		if s.tools == nil && s.orchestration != nil {
			s.tools = atool.New(s.orchestration)
			s.tools.SetDebugLogger(w)
		}
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

func WithConversionClient(client conversation.Client) Option {
	return func(s *Service) {
		s.convClient = client
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

// WithTools sets a custom atool registry.
func WithTools(tools tool.Registry) Option {
	return func(s *Service) {
		s.tools = tools
	}
}

// WithoutHotSwap disables automatic workspace hot-reload. Use this option for
// deterministic production runs where configuration should not change while
// the process is running.
func WithoutHotSwap() Option {
	return func(s *Service) {
		s.hotSwapDisabled = true
	}
}
