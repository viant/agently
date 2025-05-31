package agent

import (
	"github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/extension/fluxor/llm/augmenter"
	"github.com/viant/agently/genai/extension/fluxor/llm/core"
	"reflect"
	"strings"
	"time"

	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/tool"
	"github.com/viant/fluxor"
	"github.com/viant/fluxor/model/types"
)

// Option customises Service instances.
type Option func(*Service)

// WithSummaryPrompt overrides the default conversation summarisation prompt.
func WithSummaryPrompt(prompt string) Option {
    return func(s *Service) { s.summaryPrompt = prompt }
}

const (
    name                   = "llm/agent"
    defaultSummaryThreshold = 20
    defaultLastN           = 10
    defaultWorkflowTimeout = time.Minute
)

// Service provides functionality for working with agents
// Service provides functionality for working with agents

type Service struct {
	llm         *core.Service
	agentFinder agent.Finder
	augmenter   *augmenter.Service
	history     memory.History
	registry    *tool.Registry
	// Runtime is the shared fluxor workflow runtime for orchestration
	runtime *fluxor.Runtime

	// configurable parameters
	summaryThreshold int
	lastN            int
	workflowTimeout  time.Duration

	// template for conversation summarization; if empty a default English
	// prompt is used. It can reference ${conversation} placeholder.
	summaryPrompt string
}

// SetRuntime sets the fluxor runtime for orchestration
func (s *Service) SetRuntime(rt *fluxor.Runtime) {
	s.runtime = rt
}

// New creates a new agent service instance with the given tool registry and fluxor runtime
func New(
    llm *core.Service,
    agentFinder agent.Finder,
    augmenter *augmenter.Service,
    registry *tool.Registry,
    runtime *fluxor.Runtime,
    history memory.History,
    opts ...Option,
) *Service {
    srv := &Service{
        llm:              llm,
        agentFinder:      agentFinder,
        augmenter:        augmenter,
        history:          history,
        registry:         registry,
        runtime:          runtime,
        summaryThreshold: defaultSummaryThreshold,
        lastN:            defaultLastN,
        workflowTimeout:  defaultWorkflowTimeout,
    }

    for _, o := range opts {
        o(srv)
    }
    return srv
}

// Name returns the service name
func (s *Service) Name() string {
	return name
}

// Methods returns the service methods
func (s *Service) Methods() types.Signatures {
	return []types.Signature{
		{
			Name:   "query",
			Input:  reflect.TypeOf(&QueryInput{}),
			Output: reflect.TypeOf(&QueryOutput{}),
		},
	}
}

// Method returns the specified method
func (s *Service) Method(name string) (types.Executable, error) {
	switch strings.ToLower(name) {
	case "query":
		return s.query, nil
	default:
		return nil, types.NewMethodNotFoundError(name)
	}
}
