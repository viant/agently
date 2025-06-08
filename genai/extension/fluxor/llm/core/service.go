package core

import (
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/tool"
	"github.com/viant/fluxor/model/types"
	"io"
	"reflect"
	"strings"
)

const Name = "llm/core"

type Service struct {
	registry tool.Registry

	llmFinder    llm.Finder
	modelMatcher llm.Matcher
	defaultModel string

	logWriter io.Writer
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
			Name:   "stream",
			Input:  reflect.TypeOf(&GenerateInput{}),
			Output: reflect.TypeOf(&StreamOutput{}),
		},
	}
}

// SetLogger sets the writer used to log LLM requests and responses.
func (s *Service) SetLogger(w io.Writer) {
	s.logWriter = w
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
	case "stream":
		return s.stream, nil
	default:
		return nil, types.NewMethodNotFoundError(name)
	}
}

// New creates a new extractor service
func New(finder llm.Finder, registry tool.Registry, defaultModel string) *Service {
	return &Service{llmFinder: finder, registry: registry, defaultModel: defaultModel}
}
