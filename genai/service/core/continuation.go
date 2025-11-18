package core

import (
	"context"
	"strings"

	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/prompt"
)

// BuildContinuationRequest constructs a continuation request by selecting the latest
// assistant response anchor (resp.id) and including only tool-call messages that
// map to that anchor.
func (s *Service) BuildContinuationRequest(ctx context.Context, req *llm.GenerateRequest, history *prompt.History) *llm.GenerateRequest {
	var conversationID string
	if meta, ok := memory.TurnMetaFromContext(ctx); ok {
		conversationID = meta.ConversationID
	}
	if conversationID == "" {
		conversationID = memory.ConversationIDFromContext(ctx)
	}

	anchor := history.LastResponse
	if req == nil || strings.TrimSpace(conversationID) == "" || anchor == nil || !anchor.IsValid() || len(history.Traces) == 0 {
		return nil
	}

	// Anchor derived from binding History.LastResponse
	anchorTime := anchor.At
	anchorID := strings.TrimSpace(anchor.ID)

	// Collect tool-call messages mapped to this anchor or created after anchor time
	// Build allowed opIds by checking traces entries of Kind=="op"

	var selected llm.Messages
	for _, m := range req.Messages {
		if m.ToolCallId != "" {
			key := prompt.KindToolCall.Key(m.ToolCallId)
			trace, ok := history.Traces[key]
			if !ok || trace.ID != anchorID {
				continue
			}
			selected.Append(m)
			continue
		}

		if m.Content != "" {
			if llm.MessageRole(m.Role) != llm.RoleUser {
				continue
			}
			key := prompt.KindContent.Key(m.Content)
			trace, ok := history.Traces[key]
			if !ok || trace.At.Before(anchorTime) || trace.At.Equal(anchorTime) {
				continue
			}
			selected.Append(m)
		}
	}
	if len(selected) == 0 {
		return nil
	}

	// Build continuation request with selected tool-call messages
	continuationRequest := &llm.GenerateRequest{}
	if req.Options != nil {
		opts := *req.Options
		continuationRequest.Options = &opts
	}
	continuationRequest.Messages = append(continuationRequest.Messages, selected...)
	continuationRequest.PreviousResponseID = anchorID
	return continuationRequest
}
