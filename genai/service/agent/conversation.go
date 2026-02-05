package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	apiconv "github.com/viant/agently/client/conversation"
	convw "github.com/viant/agently/pkg/agently/conversation/write"
)

// ConversationMetadata is a typed representation of conversation metadata.
// It preserves unknown fields during round trips.
type ConversationMetadata struct {
	Tools   []string                   `json:"tools,omitempty"`
	Context map[string]interface{}     `json:"context,omitempty"`
	Extra   map[string]json.RawMessage `json:"-"`
}

func (m *ConversationMetadata) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	m.Extra = map[string]json.RawMessage{}
	for k, v := range raw {
		switch k {
		case "tools":
			var tools []string
			if err := json.Unmarshal(v, &tools); err == nil {
				m.Tools = tools
			}
		case "context":
			var ctx map[string]interface{}
			if err := json.Unmarshal(v, &ctx); err == nil {
				m.Context = ctx
			}
		default:
			m.Extra[k] = v
		}
	}
	return nil
}

func (m ConversationMetadata) MarshalJSON() ([]byte, error) {
	out := map[string]json.RawMessage{}
	if len(m.Tools) > 0 {
		if b, err := json.Marshal(m.Tools); err == nil {
			out["tools"] = b
		} else {
			return nil, err
		}
	}
	if len(m.Context) > 0 {
		if b, err := json.Marshal(m.Context); err == nil {
			out["context"] = b
		} else {
			return nil, err
		}
	}
	for k, v := range m.Extra {
		if _, exists := out[k]; !exists {
			out[k] = v
		}
	}
	return json.Marshal(out)
}

// ensureConversation loads or persists per-conversation defaults via domain store (or legacy history fallback).
func (s *Service) ensureConversation(ctx context.Context, input *QueryInput) error {
	convID := input.ConversationID
	if convID == "" {
		convID = uuid.New().String()
		input.ConversationID = convID
	}
	if s.conversation == nil {
		return fmt.Errorf("conversation API not configured")
	}
	var (
		defaultModel *string
		agentIDPtr   *string
		metadata     *string
		exists       bool
	)
	aConversation, err := s.conversation.GetConversation(ctx, convID)
	if err != nil {
		return fmt.Errorf("failed to load conversation: %w", err)
	}

	if aConversation == nil || aConversation.UpdatedAt == nil && aConversation.CreatedAt.Equal(*aConversation.LastActivity) {
		input.IsNewConversation = true
	} else {
		input.IsNewConversation = false
	}

	if exists = aConversation != nil; exists {
		defaultModel = aConversation.DefaultModel
		agentIDPtr = aConversation.AgentId
		metadata = aConversation.Metadata
	}

	// Derive model when not provided: fall back to conversation default model only.
	if input.ModelOverride == "" {
		if defaultModel != nil && strings.TrimSpace(*defaultModel) != "" {
			input.ModelOverride = *defaultModel
		}
	}

	// Tool metadata: read once, then decide to populate input
	var meta ConversationMetadata
	if metadata != nil && strings.TrimSpace(*metadata) != "" {
		_ = json.Unmarshal([]byte(*metadata), &meta)
	}
	// Initialize attachment usage from metadata if available
	if raw, ok := meta.Extra["attachments"]; ok && s.llm != nil {
		var aux struct {
			Bytes int64 `json:"bytes"`
		}
		if err := json.Unmarshal(raw, &aux); err == nil && aux.Bytes > 0 {
			s.llm.SetAttachmentUsage(convID, aux.Bytes)
		}
	}
	autoSelectTools := false
	if input.AutoSelectTools != nil {
		autoSelectTools = *input.AutoSelectTools
	} else if s.defaults != nil {
		autoSelectTools = s.defaults.ToolAutoSelection.Enabled
	}

	// Only hydrate from stored tool allow-list when tool auto-selection is not enabled.
	// When auto-selection is enabled we want tool routing to decide bundles/tools per turn.
	if !autoSelectTools && len(input.ToolsAllowed) == 0 && input.ToolsAllowed == nil {
		applyMetaTools := true
		storedAgent := ""
		if agentIDPtr != nil {
			storedAgent = strings.TrimSpace(*agentIDPtr)
		}
		reqAgent := strings.TrimSpace(input.AgentID)
		if storedAgent != "" {
			if isAutoAgentRef(storedAgent) {
				applyMetaTools = false
			} else if reqAgent != "" && !strings.EqualFold(reqAgent, storedAgent) {
				applyMetaTools = false
			}
		} else if reqAgent != "" {
			applyMetaTools = false
		}
		if applyMetaTools && len(meta.Tools) > 0 {
			input.ToolsAllowed = append([]string(nil), meta.Tools...)
		}
	}

	chosenAgentID := ""
	if strings.TrimSpace(input.AgentID) != "" {
		chosenAgentID = input.AgentID
	} else if input.Agent != nil && strings.TrimSpace(input.Agent.ID) != "" {
		chosenAgentID = input.Agent.ID
	}
	if chosenAgentID == "" {
		if agentIDPtr != nil && strings.TrimSpace(*agentIDPtr) != "" {
			input.AgentID = *agentIDPtr
		}
	}

	// Prepare a single patch with all required changes
	patch := &convw.Conversation{Has: &convw.ConversationHas{}}
	patch.SetId(convID)
	needsPatch := false

	if !exists {
		// Default new agent-created conversations to private for privacy.
		patch.SetVisibility(convw.VisibilityPrivate)
		needsPatch = true
	}
	if strings.TrimSpace(input.ModelOverride) != "" {
		patch.SetDefaultModel(strings.TrimSpace(input.ModelOverride))
		needsPatch = true
	}
	// Intentionally do not patch agent name; conversation stores agent_id separately.
	// Persist explicit tool allow-list only when tool auto-selection is not enabled.
	// Otherwise we would pin a per-turn decision into conversation metadata.
	if !autoSelectTools && len(input.ToolsAllowed) > 0 { // update tools metadata only when provided
		meta.Tools = append([]string(nil), input.ToolsAllowed...)
		if b, err := json.Marshal(meta); err == nil {
			patch.SetMetadata(string(b))
			needsPatch = true
		} else {
			return fmt.Errorf("failed to marshal tools metadata: %w", err)
		}
	}
	if needsPatch {
		if s.conversation == nil {
			return fmt.Errorf("conversation client not configured")
		}
		mc := convw.Conversation(*patch)
		if err := s.conversation.PatchConversations(ctx, (*apiconv.MutableConversation)(&mc)); err != nil {
			if !exists {
				return fmt.Errorf("failed to create conversation: %w", err)
			}
			return fmt.Errorf("failed to update conversation: %w", err)
		}
	}
	return nil
}

// updateAttachmentUsageMetadata merges/updates conversation metadata with attachments bytes
func (s *Service) updateAttachmentUsageMetadata(ctx context.Context, convID string, used int64) error {
	if s.conversation == nil || strings.TrimSpace(convID) == "" {
		return nil
	}
	cv, err := s.conversation.GetConversation(ctx, convID)
	if err != nil {
		return err
	}
	var meta ConversationMetadata
	if cv != nil && cv.Metadata != nil && strings.TrimSpace(*cv.Metadata) != "" {
		_ = json.Unmarshal([]byte(*cv.Metadata), &meta)
	}
	// preserve existing extra keys, update attachments
	if meta.Extra == nil {
		meta.Extra = map[string]json.RawMessage{}
	}
	attObj := struct {
		Bytes int64 `json:"bytes"`
	}{Bytes: used}
	if b, err := json.Marshal(attObj); err == nil {
		meta.Extra["attachments"] = b
	}
	mb, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	w := &convw.Conversation{Has: &convw.ConversationHas{}}
	w.SetId(convID)
	w.SetMetadata(string(mb))
	mw := convw.Conversation(*w)
	return s.conversation.PatchConversations(ctx, (*apiconv.MutableConversation)(&mw))
}

// updatedConversationContext saves qi.Context to conversation metadata (or history meta) after validation.
func (s *Service) updatedConversationContext(ctx context.Context, convID string, qi *QueryInput) error {
	if convID == "" || len(qi.Context) == 0 {
		return nil
	}
	if s.conversation == nil {
		return fmt.Errorf("conversation API not configured")
	}
	var metaSrc string
	cv, err := s.conversation.GetConversation(ctx, convID)
	if err != nil {
		return fmt.Errorf("failed to load conversation: %w", err)
	}
	if cv != nil && cv.Metadata != nil && strings.TrimSpace(*cv.Metadata) != "" {
		metaSrc = *cv.Metadata
	}
	var meta ConversationMetadata
	if metaSrc != "" {
		_ = json.Unmarshal([]byte(metaSrc), &meta)
	}
	// copy context
	ctxCopy := map[string]interface{}{}
	for k, v := range qi.Context {
		ctxCopy[k] = v
	}
	meta.Context = ctxCopy
	if b, err := json.Marshal(meta); err == nil {
		w := &convw.Conversation{Has: &convw.ConversationHas{}}
		w.SetId(convID)
		w.SetMetadata(string(b))
		if s.conversation == nil {
			return fmt.Errorf("conversation client not configured")
		}
		mw := convw.Conversation(*w)
		if err := s.conversation.PatchConversations(ctx, (*apiconv.MutableConversation)(&mw)); err != nil {
			return fmt.Errorf("failed to persist conversation context: %w", err)
		}
	} else {
		return fmt.Errorf("failed to marshal conversation context: %w", err)
	}
	return nil
}
