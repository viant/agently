package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	convw "github.com/viant/agently/internal/dao/conversation/write"
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
	if s.convAPI == nil {
		return fmt.Errorf("conversation API not configured")
	}
	var (
		defaultModel *string
		agentName    *string
		metadata     *string
		exists       bool
	)
	aConversation, err := s.convAPI.Get(ctx, convID)
	if err != nil {
		return fmt.Errorf("failed to load conversation: %w", err)
	}
	if exists = aConversation != nil; exists {
		defaultModel = aConversation.DefaultModel
		agentName = aConversation.AgentName
		metadata = aConversation.Metadata
	}

	// Derive model when not provided: fall back to conversation default model only.
	if input.ModelOverride == "" {
		if defaultModel != nil && strings.TrimSpace(*defaultModel) != "" {
			input.ModelOverride = *defaultModel
		}
	}

	// Tools metadata: read once, then decide to populate input
	var meta ConversationMetadata
	if metadata != nil && strings.TrimSpace(*metadata) != "" {
		_ = json.Unmarshal([]byte(*metadata), &meta)
	}
	if len(input.ToolsAllowed) == 0 {
		if len(meta.Tools) > 0 {
			input.ToolsAllowed = append([]string(nil), meta.Tools...)
		}
	}

	chosenAgent := ""
	if strings.TrimSpace(input.AgentName) != "" {
		chosenAgent = input.AgentName
	} else if input.Agent != nil && strings.TrimSpace(input.Agent.Name) != "" {
		chosenAgent = input.Agent.Name
	}
	if chosenAgent == "" {
		if agentName != nil && strings.TrimSpace(*agentName) != "" {
			input.AgentName = *agentName
		}
	}

	// Prepare a single patch with all required changes
	patch := &convw.Conversation{Has: &convw.ConversationHas{}}
	patch.SetId(convID)
	needsPatch := false

	if !exists {
		patch.SetVisibility(convw.VisibilityPublic)
		needsPatch = true
	}
	if strings.TrimSpace(input.ModelOverride) != "" {
		patch.SetDefaultModel(strings.TrimSpace(input.ModelOverride))
		needsPatch = true
	}
	if chosenAgent != "" { // set agent name when provided
		patch.SetAgentName(chosenAgent)
		needsPatch = true
	}
	if len(input.ToolsAllowed) > 0 { // update tools metadata only when provided
		meta.Tools = append([]string(nil), input.ToolsAllowed...)
		if b, err := json.Marshal(meta); err == nil {
			patch.SetMetadata(string(b))
			needsPatch = true
		} else {
			return fmt.Errorf("failed to marshal tools metadata: %w", err)
		}
	}
	if needsPatch {
		if _, err := s.store.Conversations().Patch(ctx, patch); err != nil {
			if !exists {
				return fmt.Errorf("failed to create conversation: %w", err)
			}
			return fmt.Errorf("failed to update conversation: %w", err)
		}
	}
	return nil
}

// updatedConversationContext saves qi.Context to conversation metadata (or history meta) after validation.
func (s *Service) updatedConversationContext(ctx context.Context, convID string, qi *QueryInput) error {
	if convID == "" || len(qi.Context) == 0 {
		return nil
	}
	if s.convAPI == nil {
		return fmt.Errorf("conversation API not configured")
	}
	var metaSrc string
	cv, err := s.convAPI.Get(ctx, convID)
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
		if _, err := s.store.Conversations().Patch(ctx, w); err != nil {
			return fmt.Errorf("failed to persist conversation context: %w", err)
		}
	} else {
		return fmt.Errorf("failed to marshal conversation context: %w", err)
	}
	return nil
}
