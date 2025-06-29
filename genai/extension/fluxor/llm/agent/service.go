package agent

import (
	"github.com/viant/afs"
	"github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/executor/config"
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

// WithWorkflowTimeout sets the maximum duration the Query handler will wait
// for an orchestration workflow to complete before giving up and returning a
// partial result.  Passing 0 or a negative duration leaves the current value
// unchanged.
func WithWorkflowTimeout(d time.Duration) Option {
	return func(s *Service) {
		if d > 0 {
			s.workflowTimeout = d
		}
	}
}

// WithSummaryPrompt overrides the default conversation summarisation prompt.
func WithSummaryPrompt(prompt string) Option {
	return func(s *Service) { s.summaryPrompt = prompt }
}

const (
	name                    = "llm/agent"
	defaultSummaryThreshold = 20
	defaultLastN            = 10
	defaultWorkflowTimeout  = 15 * time.Minute
)

// Service provides functionality for working with agents
// Service provides functionality for working with agents

type Service struct {
	llm         *core.Service
	agentFinder agent.Finder
	augmenter   *augmenter.Service
	history     memory.History
	traceStore  *memory.ExecutionStore
	registry    tool.Registry
	// Runtime is the shared fluxor workflow runtime for orchestration
	runtime *fluxor.Runtime

	// configurable parameters
	summaryThreshold int
	lastN            int
	workflowTimeout  time.Duration
	fs               afs.Service

	// template for conversation summarization; if empty a default English
	// prompt is used. It can reference ${conversation} placeholder.
	summaryPrompt string
	defaults      *config.Defaults
}

// SetRuntime sets the fluxor runtime for orchestration
func (s *Service) SetRuntime(rt *fluxor.Runtime) {
	s.runtime = rt
}

// New creates a new agent service instance with the given tool registry and fluxor runtime
func New(llm *core.Service, agentFinder agent.Finder, augmenter *augmenter.Service, registry tool.Registry, runtime *fluxor.Runtime, history memory.History, traceStore *memory.ExecutionStore,
	defaults *config.Defaults, opts ...Option) *Service {
	srv := &Service{
		defaults:         defaults,
		llm:              llm,
		agentFinder:      agentFinder,
		augmenter:        augmenter,
		history:          history,
		traceStore:       traceStore,
		registry:         registry,
		runtime:          runtime,
		summaryThreshold: defaultSummaryThreshold,
		lastN:            defaultLastN,
		workflowTimeout:  defaultWorkflowTimeout,
		fs:               afs.New(),
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
