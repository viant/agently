package executor

import (
	"github.com/viant/agently/genai/agent"
	modelprovider "github.com/viant/agently/genai/llm/provider"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/tool"
	"github.com/viant/fluxor"
	"github.com/viant/fluxor/service/meta"
)

type Option func(config *Service)

func WithConfig(config *Config) Option {
	return func(s *Service) {
		s.config = config
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
		s.workflow.Options = append(s.workflow.Options, option...)
	}
}

func WithModelConfig(providers ...*modelprovider.Config) Option {
	return func(s *Service) {
		if s.config.Model == nil {
			s.config.Model = &Group[*modelprovider.Config]{}
		}
		s.config.Model.Items = append(s.config.Model.Items, providers...)
	}

}

// WithAgents sets a custom agent registry.
func WithAgents(agents ...*agent.Agent) Option {
	return func(s *Service) {
		if s.config.Agent == nil {
			s.config.Agent = &Group[*agent.Agent]{}
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
