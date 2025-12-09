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
	anchorID := strings.TrimSpace(anchor.ID)

	// Collect tool-call messages mapped to this anchor. User messages
	// are already part of the anchored context and do not participate
	// in continuation-by-anchor.
	var selected llm.Messages
	for _, m := range req.Messages {

		if len(m.ToolCalls) > 0 {
			filtered := filterToolCallsByAnchor(m.ToolCalls, history, anchorID)
			if len(filtered) == 0 {
				continue
			}
			copyMsg := m
			copyMsg.ToolCalls = filtered
			selected.Append(copyMsg)
			continue
		}

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

			if !ok || trace.At.Before(anchor.At) || trace.At.Equal(anchor.At) {
				continue
			}

			selected.Append(m)
			continue
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

func filterToolCallsByAnchor(toolCalls []llm.ToolCall, history *prompt.History, anchorID string) []llm.ToolCall {
	if len(toolCalls) == 0 || history == nil || anchorID == "" {
		return nil
	}
	var filtered []llm.ToolCall
	for _, call := range toolCalls {
		key := prompt.KindToolCall.Key(call.ID)
		trace, ok := history.Traces[key]
		if !ok || trace.ID != anchorID {
			continue
		}
		filtered = append(filtered, call)
	}
	return filtered
}
