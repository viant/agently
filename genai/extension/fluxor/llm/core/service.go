package core

import (
	"context"
	"reflect"
	"strings"

	"github.com/viant/afs"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/tool"
	domainrec "github.com/viant/agently/internal/domain/recorder"
	"github.com/viant/fluxor/model/types"
)

const Name = "llm/core"

type Service struct {
	registry     tool.Registry
	llmFinder    llm.Finder
	modelMatcher llm.Matcher
	fs           afs.Service
	recorder     domainrec.Recorder
}

func (s *Service) ModelFinder() llm.Finder {
	return s.llmFinder
}

func (s *Service) ModelMatcher() llm.Matcher {
	return s.modelMatcher
}

// ToolDefinitions returns every tool definition registered in the tool
// registry.  The slice may be empty when no registry is configured (unit tests
// or mis-configuration).
func (s *Service) ToolDefinitions() []llm.ToolDefinition {
	if s == nil || s.registry == nil {
		return nil
	}
	return s.registry.Definitions()
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
	}
}

// Method returns the specified method
func (s *Service) Method(name string) (types.Executable, error) {
	switch strings.ToLower(name) {
	case "generate":
		return s.generate, nil
	default:
		return nil, types.NewMethodNotFoundError(name)
	}
}

// New creates a new extractor service
func New(finder llm.Finder, registry tool.Registry, recorder domainrec.Recorder) *Service {
	matcher, _ := finder.(llm.Matcher)
	return &Service{llmFinder: finder, registry: registry, recorder: recorder, fs: afs.New(), modelMatcher: matcher}
}

// ModelImplements reports whether a given model supports a feature.
// When modelName is empty or not found, it returns false.
func (s *Service) ModelImplements(ctx context.Context, modelName, feature string) bool {
	if s == nil || s.llmFinder == nil || strings.TrimSpace(modelName) == "" || strings.TrimSpace(feature) == "" {
		return false
	}
	model, _ := s.llmFinder.Find(ctx, modelName)
	if model == nil {
		return false
	}
	return model.Implements(feature)
}

func (s *Service) SetRecorder(r domainrec.Recorder) { s.recorder = r }
