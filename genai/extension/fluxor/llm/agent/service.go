// Package agent coordinates an agent turn across multiple responsibilities
// (message recording, turn lifecycle, model-call capture, usage totals).
//
// Dependency policy:
//   - This service intentionally depends on the aggregated domain writer
//     (writer.Recorder) because it uses several facets together.
//   - Favor passing the single aggregated Recorder rather than multiple small
//     interfaces to keep wiring simple and debugging easier.
//   - If needs change to a single facet (unlikely here), consider narrowing, but
//     avoid adding multiple separate parameters.
package agent

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/viant/afs"
	"github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/executor/config"
	"github.com/viant/agently/genai/extension/fluxor/llm/augmenter"
	"github.com/viant/agently/genai/extension/fluxor/llm/core"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/tool"
	d "github.com/viant/agently/internal/domain"
	recorder "github.com/viant/agently/internal/domain/recorder"
	"github.com/viant/fluxor"
	"github.com/viant/fluxor/model/types"
)

// Option customises Service instances.
type Option func(*Service)

// -------------------- Run action types ----------------------------------

// RunInput describes the parameters for the new "run" executable that
// launches a sub-agent in a child conversation.

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

// WithSummaryModel overrides the default model used when creating summaries
// for child-conversation link messages emitted by the internal "run" action.
func WithSummaryModel(model string) Option {
	return func(s *Service) { s.runSummaryModel = strings.TrimSpace(model) }
}

// WithSummaryLastN sets how many recent messages are included when summarising
// a child conversation for the link preview inserted into the parent thread.
// A value <= 0 leaves the previous configuration untouched.
func WithSummaryLastN(n int) Option {
	return func(s *Service) {
		if n > 0 {
			s.lastN = n
		}
	}
}

// WithDomainStore injects a domain.Store for conversation-level reads/writes.
func WithDomainStore(store d.Store) Option {
	return func(s *Service) { s.domainStore = store }
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

	// run-action summary settings
	runSummaryModel string

	// template for conversation summarization; if empty a default English
	// prompt is used. It can reference ${conversation} placeholder.
	summaryPrompt string
	defaults      *config.Defaults

	// domain writer for shadow writes.
	// Aggregated Recorder is used here because this service records messages,
	// turns, model calls, and usage. A single dependency avoids interface
	// sprawl and simplifies debugging.
	domainWriter recorder.Recorder
	domainStore  d.Store
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
		{
			Name:   "run",
			Input:  reflect.TypeOf(&RunInput{}),
			Output: reflect.TypeOf(&RunOutput{}),
		},
	}
}

// Method returns the specified method
func (s *Service) Method(name string) (types.Executable, error) {
	switch strings.ToLower(name) {
	case "query":
		return s.query, nil
	case "run":
		return s.run, nil
	default:
		return nil, types.NewMethodNotFoundError(name)
	}
}

// Query is a public helper that allows callers outside Fluxor runtime to run
// an agent turn (or its orchestration workflow) programmatically. It wraps
// the internal query executable so that other actions (e.g. agent.run) can
// reuse existing logic without going through reflection-based invocation.
func (s *Service) Query(ctx context.Context, in *QueryInput) (*QueryOutput, error) {
	if s == nil {
		return nil, fmt.Errorf("agent service is nil")
	}
	if in == nil {
		return nil, fmt.Errorf("query input is nil")
	}
	var out QueryOutput
	if err := s.query(ctx, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// run implements the executable registered under "run". It spawns a child
// conversation, delegates the actual agent turn via Query(), then writes a
// link message (optionally with summary) into the parent thread.
// WithDomainWriter injects a domain writer used for shadow writes.
// WithDomainWriter injects the aggregated Recorder. Prefer Recorder here over
// multiple narrow interfaces because the agent coordinates multiple facets.
func WithDomainWriter(w recorder.Recorder) Option { return func(s *Service) { s.domainWriter = w } }
