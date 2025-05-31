package exec

import (
	"github.com/viant/agently/genai/extension/fluxor/llm/core"
	"github.com/viant/agently/genai/tool"
	"github.com/viant/fluxor/model/types"
	"reflect"
	"strings"
)

const name = "llm/exec"

type Service struct {
	registry     *tool.Registry
	llm          *core.Service
	defaultModel string
	// MaxRetries is the default number of attempts per tool call if not overridden in PlanStep.Retries.
	maxRetries int
	maxSteps   int
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
func New(llm *core.Service, registry *tool.Registry, defaultModel string) *Service {
	return &Service{llm: llm, registry: registry, defaultModel: defaultModel, maxRetries: 3, maxSteps: 1000}
}
