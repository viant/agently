// Package exec persists tool-call operations originating from workflow steps.
//
// Dependency policy:
//   - Although this service currently records only tool calls, it accepts the
//     aggregated writer.Recorder for consistency with other services and simpler
//     wiring. Recorder also satisfies the narrower contract used here.
package exec

import (
	"reflect"
	"strings"

	"github.com/viant/agently/genai/extension/fluxor/llm/core"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/tool"
	"github.com/viant/agently/internal/domain/recorder"
	"github.com/viant/fluxor/model/types"
	"github.com/viant/fluxor/service/approval"
)

const name = "llm/exec"

type Service struct {
	registry        tool.Registry
	llm             *core.Service
	defaultModel    string
	approvalService approval.Service
	traceStore      *memory.ExecutionStore
	// MaxRetries is the default number of attempts per tool call if not overridden in PlanStep.Retries.
	maxRetries int
	maxSteps   int

	// recorder for shadow writes (aggregated Recorder for consistency).
	recorder recorder.Recorder
}

// Name returns the service name
func (s *Service) Name() string {
	return name
}

// Methods returns the service methods
func (s *Service) Methods() types.Signatures {
	return []types.Signature{
		{
			Name:   "run_plan",
			Input:  reflect.TypeOf(&RunPlanInput{}),
			Output: reflect.TypeOf(&RunPlanOutput{}),
		},
		{
			Name:   "call_tool",
			Input:  reflect.TypeOf(&CallToolInput{}),
			Output: reflect.TypeOf(&CallToolOutput{}),
		},
	}
}

// Method returns the specified method
func (s *Service) Method(name string) (types.Executable, error) {
	switch strings.ToLower(name) {
	case "run_plan":
		return s.runPlan, nil
	case "call_tool":
		return s.callTool, nil
	default:
		return nil, types.NewMethodNotFoundError(name)
	}
}

// New creates a new extractor service
func New(llm *core.Service, registry tool.Registry, defaultModel string, approvalService approval.Service, traceStore *memory.ExecutionStore) *Service {
	return &Service{
		llm:             llm,
		registry:        registry,
		defaultModel:    defaultModel,
		approvalService: approvalService,
		traceStore:      traceStore,
		maxRetries:      3,
		maxSteps:        1000,
	}
}

// WithDomainWriter sets optional domain writer used for shadow writes.
// Accepts the aggregated writer.Recorder; Recorder also satisfies the
// narrower Enablement + ToolCallRecorder contract needed here.
func (s *Service) WithRecorder(w recorder.Recorder) *Service { s.recorder = w; return s }
