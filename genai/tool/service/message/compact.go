package message

// This compaction is intended only to free space for the Token‑Limit Presentation
// message during a context-limit recovery flow. It should not be exposed to the LLM
// as a general-purpose cleanup tool. Prefer LLM-driven removals via
// message:listCandidates + message:remove for normal operation.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/memory"
	agconv "github.com/viant/agently/pkg/agently/conversation"
)

type CompactInput struct {
	MaxTokens      int    `json:"maxTokens" description:"Target total token budget for retained messages."`
	Strategy       string `json:"strategy,omitempty" description:"Removal order: oldest-first (default), text-first, tool-first" choice:"oldest-first" choice:"text-first" choice:"tool-first"`
	PreservePinned bool   `json:"preservePinned,omitempty" description:"Reserved for future use (no effect in v1)."`
}

type CompactOutput struct {
	RemovedCount int `json:"removedCount"`
	FreedTokens  int `json:"freedTokens"`
	KeptTokens   int `json:"keptTokens"`
}

// compact frees enough space to satisfy a MaxTokens budget by archiving the
// minimum number of oldest items and inserting short summaries for retained
// memory. It is used internally to make room for the token-limit presentation.
func (s *Service) compact(ctx context.Context, in, out interface{}) error {
	input, ok := in.(*CompactInput)
	if !ok {
		return fmt.Errorf("invalid input")
	}
	output, ok := out.(*CompactOutput)
	if !ok {
		return fmt.Errorf("invalid output")
	}
	if s == nil || s.conv == nil {
		return fmt.Errorf("conversation client not initialised")
	}
	if input.MaxTokens <= 0 {
		return fmt.Errorf("maxTokens must be > 0")
	}
	convID := memory.ConversationIDFromContext(ctx)
	if strings.TrimSpace(convID) == "" {
		return fmt.Errorf("missing conversation id in context")
	}
	conv, err := s.conv.GetConversation(ctx, convID, apiconv.WithIncludeToolCall(true))
	if err != nil || conv == nil {
		return fmt.Errorf("failed to get conversation: %w", err)
	}

	// Compute current token usage of candidate messages
	tr := conv.GetTranscript()
	lastUserID := lastUserMessageID(tr)
	type item struct {
		msgID, typ, role, tool string
		tokens                 int
		body                   string
		m                      *agconv.MessageView
	}
	var items []item
	total := 0
	for _, t := range tr {
		if t == nil {
			continue
		}
		for _, m := range t.Message {
			if m == nil || m.Id == lastUserID || (m.Archived != nil && *m.Archived == 1) || m.Interim != 0 {
				continue
			}
			typ := strings.ToLower(strings.TrimSpace(m.Type))
			role := strings.ToLower(strings.TrimSpace(m.Role))
			var body string
			var tool string
			if m.ToolCall != nil {
				if m.ToolCall.ResponsePayload != nil && m.ToolCall.ResponsePayload.InlineBody != nil {
					body = *m.ToolCall.ResponsePayload.InlineBody
				}
				tool = strings.TrimSpace(m.ToolCall.ToolName)
			} else if typ == "text" && (role == "user" || role == "assistant") {
				if m.Content != nil {
					body = *m.Content
				}
			} else {
				continue
			}
			tokens := estimateTokens(body)
			total += tokens
			items = append(items, item{msgID: m.Id, typ: typ, role: role, tool: tool, tokens: tokens, body: body, m: m})
		}
	}
	if total <= input.MaxTokens {
		output.KeptTokens = total
		return nil
	}
	needed := total - input.MaxTokens

	// Order items per strategy
	order := strings.ToLower(strings.TrimSpace(input.Strategy))
	// Simple stable two-pass selection based on strategy
	selectAndRemove := func(pred func(it item) bool) {
		for i := 0; i < len(items) && needed > 0; i++ {
			it := items[i]
			if it.m == nil || it.msgID == lastUserID || it.tokens <= 0 {
				continue
			}
			if !pred(it) {
				continue
			}
			// Insert summary message
			sum := buildSummaryForMsg(it.m, it.tool, it.body)
			turn := ensureTurnMeta(ctx, s.conv, conv)
			if _, err := apiconv.AddMessage(ctx, s.conv, &turn, apiconv.WithRole("assistant"), apiconv.WithType("text"), apiconv.WithStatus("summary"), apiconv.WithContent(sum)); err != nil {
				continue
			}
			// Archive original and set message-level Summary
			mm := apiconv.NewMessage()
			mm.SetId(it.msgID)
			mm.SetArchived(1)
			if sum != "" {
				mm.SetSummary(sum)
			}
			if err := s.conv.PatchMessage(ctx, mm); err != nil {
				continue
			}
			output.RemovedCount++
			output.FreedTokens += it.tokens
			needed -= it.tokens
			// Mark consumed
			items[i].tokens = 0
		}
	}
	switch order {
	case "text-first":
		selectAndRemove(func(it item) bool { return it.typ == "text" && (it.role == "user" || it.role == "assistant") })
		selectAndRemove(func(it item) bool { return it.typ != "text" || it.m.ToolCall != nil })
	case "tool-first":
		selectAndRemove(func(it item) bool { return it.m.ToolCall != nil })
		selectAndRemove(func(it item) bool { return it.typ == "text" && (it.role == "user" || it.role == "assistant") })
	default: // oldest-first: we already traversed in transcript order from oldest to newest
		selectAndRemove(func(it item) bool { return true })
	}
	// Remaining kept tokens
	kept := 0
	for _, it := range items {
		kept += it.tokens
	}
	output.KeptTokens = kept
	return nil
}

func buildSummaryForMsg(m *agconv.MessageView, toolName, body string) string {
	// Text: first 100 chars; Tool: tool: args_preview (100 chars)
	if m != nil && m.ToolCall != nil {
		var args map[string]interface{}
		if m.ToolCall.RequestPayload != nil && m.ToolCall.RequestPayload.InlineBody != nil {
			raw := strings.TrimSpace(*m.ToolCall.RequestPayload.InlineBody)
			if raw != "" {
				_ = json.Unmarshal([]byte(raw), &args)
			}
		}
		argStr, _ := json.Marshal(args)
		ap := string(argStr)
		if len(ap) > 100 {
			ap = ap[:100]
		}
		return fmt.Sprintf("%s: %s", strings.TrimSpace(toolName), ap)
	}
	pv := body
	if len(pv) > 100 {
		pv = pv[:100]
	}
	return pv
}

func ensureTurnMeta(ctx context.Context, conv apiconv.Client, view *apiconv.Conversation) memory.TurnMeta {
	if tm, ok := memory.TurnMetaFromContext(ctx); ok {
		return tm
	}
	turnID := ""
	if view != nil && view.LastTurnId != nil {
		turnID = *view.LastTurnId
	}
	return memory.TurnMeta{ConversationID: view.Id, TurnID: turnID, ParentMessageID: turnID}
}

func lastUserMessageID(tr apiconv.Transcript) string {
	for i := len(tr) - 1; i >= 0; i-- {
		t := tr[i]
		if t == nil || len(t.Message) == 0 {
			continue
		}
		for j := len(t.Message) - 1; j >= 0; j-- {
			m := t.Message[j]
			if m == nil || m.Interim != 0 || m.Content == nil || strings.TrimSpace(*m.Content) == "" {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(m.Role), "user") {
				return m.Id
			}
		}
	}
	return ""
}

// estimateTokens is defined in tokens.go
