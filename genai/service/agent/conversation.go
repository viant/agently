package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	convw "github.com/viant/agently/internal/dao/conversation/write"
)

// ensureConversation loads or persists per-conversation defaults via domain store (or legacy history fallback).
func (s *Service) ensureConversation(ctx context.Context, input *QueryInput) error {
	convID := input.ConversationID
	if convID == "" {
		convID = uuid.New().String()
		input.ConversationID = convID
	}
	aConversation, err := s.store.Conversations().Get(ctx, convID)
	if err != nil {
		return fmt.Errorf("failed to load conversation: %w", err)
	}
	if aConversation == nil {
		initialConversation := &convw.Conversation{Has: &convw.ConversationHas{}}
		initialConversation.SetId(convID)
		// Default new conversations to public visibility; can be adjusted later.
		initialConversation.SetVisibility(convw.VisibilityPublic)
		// Seed basic meta from the request where available.
		if strings.TrimSpace(input.AgentName) != "" {
			initialConversation.SetAgentName(strings.TrimSpace(input.AgentName))
		}
		if strings.TrimSpace(input.ModelOverride) != "" {
			initialConversation.SetDefaultModel(strings.TrimSpace(input.ModelOverride))
		}
		if len(input.ToolsAllowed) > 0 {
			meta := map[string]any{"tools": append([]string{}, input.ToolsAllowed...)}
			if b, err := json.Marshal(meta); err == nil {
				initialConversation.SetMetadata(string(b))
			}
		}
		if _, err = s.store.Conversations().Patch(ctx, initialConversation); err != nil {
			return fmt.Errorf("failed to create conversation: %w", err)
		}
	}

	// Derive model when not provided: fall back to conversation default model only.
	if input.ModelOverride == "" {
		if aConversation != nil && aConversation.DefaultModel != nil && strings.TrimSpace(*aConversation.DefaultModel) != "" {
			input.ModelOverride = *aConversation.DefaultModel
		}
	} else {
		w := &convw.Conversation{Has: &convw.ConversationHas{}}
		w.SetId(convID)
		w.SetDefaultModel(input.ModelOverride)
		if _, err := s.store.Conversations().Patch(ctx, w); err != nil {
			return fmt.Errorf("failed to update conversation default model: %w", err)
		}
	}

	if len(input.ToolsAllowed) == 0 {
		if aConversation != nil && aConversation.Metadata != nil {
			var meta map[string]interface{}
			if err := json.Unmarshal([]byte(*aConversation.Metadata), &meta); err == nil {
				if arr, ok := meta["tools"].([]interface{}); ok && len(arr) > 0 {
					tools := make([]string, 0, len(arr))
					for _, it := range arr {
						if s, ok := it.(string); ok && strings.TrimSpace(s) != "" {
							tools = append(tools, s)
						}
					}
					if len(tools) > 0 {
						input.ToolsAllowed = tools
					}
				}
			}
		}
	} else {
		meta := map[string]interface{}{}
		if aConversation != nil && aConversation.Metadata != nil && strings.TrimSpace(*aConversation.Metadata) != "" {
			_ = json.Unmarshal([]byte(*aConversation.Metadata), &meta)
		}
		lst := make([]string, len(input.ToolsAllowed))
		copy(lst, input.ToolsAllowed)
		meta["tools"] = lst
		if b, err := json.Marshal(meta); err == nil {
			w := &convw.Conversation{Has: &convw.ConversationHas{}}
			w.SetId(convID)
			w.SetMetadata(string(b))
			if _, err := s.store.Conversations().Patch(ctx, w); err != nil {
				return fmt.Errorf("failed to update conversation tools: %w", err)
			}
		}
	}

	chosenAgent := ""
	if strings.TrimSpace(input.AgentName) != "" {
		chosenAgent = input.AgentName
	} else if input.Agent != nil && strings.TrimSpace(input.Agent.Name) != "" {
		chosenAgent = input.Agent.Name
	}
	if chosenAgent == "" {
		if aConversation != nil && aConversation.AgentName != nil && strings.TrimSpace(*aConversation.AgentName) != "" {
			input.AgentName = *aConversation.AgentName
		}
	} else {
		w := &convw.Conversation{Has: &convw.ConversationHas{}}
		w.SetId(convID)
		w.SetAgentName(chosenAgent)
		if _, err := s.store.Conversations().Patch(ctx, w); err != nil {
			return fmt.Errorf("failed to update conversation agent: %w", err)
		}
	}
	return nil
}

// updatedConversationContext saves qi.Context to conversation metadata (or history meta) after validation.
func (s *Service) updatedConversationContext(ctx context.Context, convID string, qi *QueryInput) error {
	if convID == "" || len(qi.Context) == 0 {
		return nil
	}
	cv, err := s.store.Conversations().Get(ctx, convID)
	if err != nil {
		return fmt.Errorf("failed to load conversation: %w", err)
	}
	meta := map[string]interface{}{}
	if cv != nil && cv.Metadata != nil && strings.TrimSpace(*cv.Metadata) != "" {
		_ = json.Unmarshal([]byte(*cv.Metadata), &meta)
	}
	ctxCopy := map[string]interface{}{}
	for k, v := range qi.Context {
		ctxCopy[k] = v
	}
	meta["context"] = ctxCopy
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
