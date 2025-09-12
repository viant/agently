package message

import (
	"strings"
	"time"

	plan "github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/memory"
	msgread "github.com/viant/agently/internal/dao/message/read"
	d "github.com/viant/agently/internal/domain"
)

// ToMemoryMessages converts DAO transcript views into memory.Messages slice
// suitable for v1 handlers and UI clients. It preserves IDs, parent linkage,
// roles, content, tool name, created time and (when present) aggregated
// executions for tool messages. It also maps typed elicitation when available.
func ToMemoryMessages(views []*msgread.MessageView) []memory.Message {
	out := make([]memory.Message, 0, len(views))
	// Reuse domain transcript helpers to drop interim consistently
	tr := d.Transcript(views).WithoutInterim()
	for _, v := range tr {
		if v == nil {
			continue
		}
		m := memory.Message{
			ID:             v.Id,
			ConversationID: v.ConversationID,
			Role:           v.Role,
			Content:        v.Content,
			Elicitation:    v.Elicitation,
		}
		if v.ParentID != nil {
			m.ParentID = *v.ParentID
		}
		if v.ToolName != nil {
			m.ToolName = v.ToolName
		}
		if v.CreatedAt != nil {
			m.CreatedAt = *v.CreatedAt
		} else {
			m.CreatedAt = time.Now()
		}
		// Preserve executions only for tool messages when present
		if strings.EqualFold(strings.TrimSpace(v.Role), "tool") && v.ToolCall != nil {
			tc := v.ToolCall
			st := &plan.StepOutcome{
				ID:                tc.OpID,
				Name:              tc.ToolName,
				Reason:            "tool_call",
				Success:           strings.EqualFold(strings.TrimSpace(tc.Status), "completed"),
				Error:             deref(tc.ErrorMessage),
				StartedAt:         tc.StartedAt,
				EndedAt:           tc.CompletedAt,
				RequestPayloadID:  tc.RequestPayloadID,
				ResponsePayloadID: tc.ResponsePayloadID,
			}
			if tc.StartedAt != nil && tc.CompletedAt != nil {
				st.Elapsed = tc.CompletedAt.Sub(*tc.StartedAt).Round(time.Millisecond).String()
			}
			m.Executions = []*plan.Outcome{{Steps: []*plan.StepOutcome{st}}}
		}
		// Interim flag if available (kept for parity)
		if v.Interim != nil {
			m.Interim = v.Interim
		}
		out = append(out, m)
	}
	return out
}

func deref(p *string) string {
	if p != nil {
		return *p
	}
	return ""
}
