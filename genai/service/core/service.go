package core

import (
	"context"
	"reflect"
	"strings"

	"github.com/viant/afs"
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/llm/provider/base"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/tool"
	svc "github.com/viant/agently/genai/tool/service"
)

const Name = "llm/core"

type Service struct {
	registry     tool.Registry
	llmFinder    llm.Finder
	modelMatcher llm.Matcher
	fs           afs.Service
	convClient   apiconv.Client

	// attachment usage accumulator per conversation (bytes)
	attachUsage map[string]int64

	// optional per-model preview limits for tool results (bytes)
	modelPreviewLimit map[string]int
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
func (s *Service) Methods() svc.Signatures {
	return []svc.Signature{
		{
			Name:     "generate",
			Internal: true,
			Input:    reflect.TypeOf(&GenerateInput{}),
			Output:   reflect.TypeOf(&GenerateOutput{}),
		},
	}
}

// Method returns the specified method
func (s *Service) Method(name string) (svc.Executable, error) {
	switch strings.ToLower(name) {
	case "generate":
		return s.generate, nil
	default:
		return nil, svc.NewMethodNotFoundError(name)
	}
}

// New creates a new extractor service
func New(finder llm.Finder, registry tool.Registry, convClient apiconv.Client) *Service {
	matcher, _ := finder.(llm.Matcher)
	return &Service{llmFinder: finder, registry: registry, convClient: convClient, fs: afs.New(), modelMatcher: matcher, attachUsage: map[string]int64{}}
}

// resolveToolCallTraces builds a map of tool op_id -> TraceID (provider response.id)
// by reading the conversation transcript with tool calls included. Errors are
// swallowed and result in an empty map to keep core generation tolerant.
func (s *Service) resolveToolCallTraces(ctx context.Context, conversationID string) map[string]string {
	out := map[string]string{}
	if s == nil || s.convClient == nil || strings.TrimSpace(conversationID) == "" {
		return out
	}
	svc := apiconv.NewService(s.convClient)
	resp, err := svc.Get(ctx, apiconv.GetRequest{Id: conversationID, IncludeToolCall: true})
	if err != nil || resp == nil || resp.Conversation == nil {
		return out
	}
	tr := resp.Conversation.GetTranscript()
	for _, turn := range tr {
		if turn == nil || turn.Message == nil {
			continue
		}
		for _, m := range turn.Message {
			if m == nil || m.ToolCall == nil {
				continue
			}
			opID := strings.TrimSpace(m.ToolCall.OpId)
			if opID == "" {
				continue
			}
			if m.ToolCall.TraceId != nil {
				if v := strings.TrimSpace(*m.ToolCall.TraceId); v != "" {
					out[opID] = v
				}
			}
		}
	}
	return out
}

// continuationEnabled returns true when the model supports continuation by response id
// and the request options did not disable it explicitly.
func continuationEnabled(model llm.Model, opts *llm.Options) bool {
	if model == nil {
		return false
	}
	if !model.Implements(base.SupportsContinuationByResponseID) {
		return false
	}
	if opts == nil || opts.ContinuationEnabled == nil {
		return true
	}
	return *opts.ContinuationEnabled
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

func (s *Service) SetConversationClient(c apiconv.Client) { s.convClient = c }

// SetModelPreviewLimits sets per-model preview byte limits used by binding to trim tool results.
func (s *Service) SetModelPreviewLimits(m map[string]int) { s.modelPreviewLimit = m }

// ModelToolPreviewLimit returns the preview limit in bytes for a model or 0 when not configured.
func (s *Service) ModelToolPreviewLimit(model string) int {
	if s == nil || s.modelPreviewLimit == nil || strings.TrimSpace(model) == "" {
		return 0
	}
	return s.modelPreviewLimit[model]
}

// setModelCallStatus best-effort patches model_call.status for the current message.
// It requires a recorder observer to have created a message id earlier in the call.
func (s *Service) setModelCallStatus(ctx context.Context, status string) {
	if s == nil || s.convClient == nil || strings.TrimSpace(status) == "" {
		return
	}
	msgID := strings.TrimSpace(memory.ModelMessageIDFromContext(ctx))
	if msgID == "" {
		return
	}
	upd := apiconv.NewModelCall()
	upd.SetMessageID(msgID)
	upd.SetStatus(status)
	// best-effort; ignore error to not affect retry timing
	_ = s.convClient.PatchModelCall(ctx, upd)
}
