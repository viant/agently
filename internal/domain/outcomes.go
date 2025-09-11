package domain

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	plan "github.com/viant/agently/genai/agent/plan"
	msgread "github.com/viant/agently/internal/dao/message/read"
)

// BuildToolOutcomes constructs a single Outcome aggregating tool step outcomes
// from the supplied transcript. It scans messages with role "tool" (or type
// "tool_op" when role is unavailable), and for each message with an attached
// ToolCall view it creates a StepOutcome. When Request/Response payload IDs are
// present and inline bodies are available, it populates StepOutcome.Request and
// StepOutcome.Response with those JSON bodies. The function returns the
// aggregated Outcome or an error when payload retrieval fails.
func BuildToolOutcomes(ctx context.Context, store Store, transcript Transcript) (*plan.Outcome, error) {
	if store == nil {
		return &plan.Outcome{}, nil
	}
	// Filter messages likely representing tool executions
	tools := transcript.Filter(func(v *msgread.MessageView) bool {
		if v == nil || v.IsInterim() {
			return false
		}
		r := strings.ToLower(strings.TrimSpace(v.Role))
		if r == "tool" {
			return true
		}
		// Some providers use role=assistant with type=tool_op to emit tool results
		return strings.ToLower(strings.TrimSpace(v.Type)) == "tool_op"
	})

	out := &plan.Outcome{}
	for _, v := range tools {
		if v == nil || v.ToolCall == nil {
			continue
		}
		tc := v.ToolCall
		st := &plan.StepOutcome{
			ID:     tc.OpID,
			Name:   tc.ToolName,
			Reason: v.Content,
			// Mirror basic execution status fields
			Success: strings.ToLower(strings.TrimSpace(tc.Status)) == "completed",
			Error:   derefS(tc.ErrorMessage),
		}
		// Timestamps / elapsed
		st.StartedAt = tc.StartedAt
		st.EndedAt = tc.CompletedAt
		if tc.StartedAt != nil && tc.CompletedAt != nil {
			st.Elapsed = tc.CompletedAt.Sub(*tc.StartedAt).Round(time.Millisecond).String()
		}

		// Attach payload IDs for lazy clients
		st.RequestPayloadID = tc.RequestPayloadID
		st.ResponsePayloadID = tc.ResponsePayloadID

		// Inline request/response JSON bodies when available
		if tc.RequestPayloadID != nil && *tc.RequestPayloadID != "" {
			pv, err := store.Payloads().Get(ctx, *tc.RequestPayloadID)
			if err != nil {
				return nil, err
			}
			if pv != nil && pv.InlineBody != nil && len(*pv.InlineBody) > 0 {
				st.Request = json.RawMessage(*pv.InlineBody)
			}
		}
		if tc.ResponsePayloadID != nil && *tc.ResponsePayloadID != "" {
			pv, err := store.Payloads().Get(ctx, *tc.ResponsePayloadID)
			if err != nil {
				return nil, err
			}
			if pv != nil && pv.InlineBody != nil && len(*pv.InlineBody) > 0 {
				st.Response = json.RawMessage(*pv.InlineBody)
			}
		}
		out.Steps = append(out.Steps, st)
	}
	return out, nil
}

// helpers ------------------------------------------------------------------

func derefS(p *string) string {
	if p != nil {
		return *p
	}
	return ""
}
