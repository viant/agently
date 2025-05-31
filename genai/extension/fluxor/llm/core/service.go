package core

import (
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/tool"
	"github.com/viant/fluxor/extension"
	"github.com/viant/fluxor/model/types"
	"reflect"
	"strings"
)

const Name = "llm/core"

type Service struct {
	registry     *tool.Registry
	llmFinder    llm.Finder
	actions      *extension.Actions
	defaultModel string
}

// Name returns the service Name
func (s *Service) Name() string {
	return Name
}

// Methods returns the service methods
func (s *Service) Methods() types.Signatures {
	return []types.Signature{
		{
			Name:   "generate",
			Input:  reflect.TypeOf(&GenerateInput{}),
			Output: reflect.TypeOf(&GenerateOutput{}),
		},
		{
			Name:   "plan",
			Input:  reflect.TypeOf(&PlanInput{}),
			Output: reflect.TypeOf(&PlanOutput{}),
		},
		{
			Name:   "rank",
			Input:  reflect.TypeOf(&RankInput{}),
			Output: reflect.TypeOf(&RankOutput{}),
		},
		{
			Name:   "finalize",
			Input:  reflect.TypeOf(&FinalizeInput{}),
			Output: reflect.TypeOf(&FinalizeOutput{}),
		},
	}
}

// Method returns the specified method
func (s *Service) Method(name string) (types.Executable, error) {
	switch strings.ToLower(name) {
	case "generate":
		return s.generate, nil
	case "plan":
		return s.plan, nil
	case "rank":
		return s.rank, nil
	case "finalize":
		return s.finalize, nil
	default:
		return nil, types.NewMethodNotFoundError(name)
	}
}

// New creates a new extractor service
func New(finder llm.Finder, registry *tool.Registry, defaultModel string) *Service {
	return &Service{llmFinder: finder, registry: registry, defaultModel: defaultModel}
}
