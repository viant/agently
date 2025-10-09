package core

import (
	"context"
	"reflect"
	"strings"

	"github.com/viant/afs"
	chstore "github.com/viant/agently/client/chat/store"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/tool"
	"github.com/viant/fluxor/model/types"
)

const Name = "llm/core"

type Service struct {
	registry     tool.Registry
	llmFinder    llm.Finder
	modelMatcher llm.Matcher
	fs           afs.Service
	convClient   chstore.Client

	// attachment usage accumulator per conversation (bytes)
	attachUsage map[string]int64
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
			Name:     "generate",
			Internal: true,
			Input:    reflect.TypeOf(&GenerateInput{}),
			Output:   reflect.TypeOf(&GenerateOutput{}),
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
func New(finder llm.Finder, registry tool.Registry, convClient chstore.Client) *Service {
	matcher, _ := finder.(llm.Matcher)
	return &Service{llmFinder: finder, registry: registry, convClient: convClient, fs: afs.New(), modelMatcher: matcher, attachUsage: map[string]int64{}}
}

// AttachmentUsage returns cumulative attachment bytes recorded for a conversation.
func (s *Service) AttachmentUsage(convID string) int64 {
	if s == nil || s.attachUsage == nil || strings.TrimSpace(convID) == "" {
		return 0
	}
	return s.attachUsage[convID]
}

// SetAttachmentUsage sets cumulative attachment bytes for a conversation.
func (s *Service) SetAttachmentUsage(convID string, used int64) {
	if s == nil || strings.TrimSpace(convID) == "" {
		return
	}
	if s.attachUsage == nil {
		s.attachUsage = map[string]int64{}
	}
	s.attachUsage[convID] = used
}

// ProviderAttachmentLimit returns the provider-configured attachment cap for the given model.
// Zero means unlimited/not enforced by this provider.
func (s *Service) ProviderAttachmentLimit(model llm.Model) int64 {
	if model == nil {
		return 0
	}
	// Default OpenAI limit when applicable: avoid importing client types; assume limit applied upstream via Agent.
	// Returning 0 keeps core enforcement neutral; agent layer enforces and persists within cap.
	return 0
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

func (s *Service) SetConversationClient(c chstore.Client) { s.convClient = c }
