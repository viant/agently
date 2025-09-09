package message

import (
	"strings"
	"time"

	"github.com/viant/agently/genai/memory"
	msgread "github.com/viant/agently/internal/dao/message/read"
	d "github.com/viant/agently/internal/domain"
)

// ToMemoryMessages converts DAO transcript views into memory.Message slice
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
		if strings.EqualFold(strings.TrimSpace(v.Role), "tool") && len(v.Executions) > 0 {
			m.Executions = v.Executions
		}
		// Interim flag if available (kept for parity)
		if v.Interim != nil {
			m.Interim = v.Interim
		}
		out = append(out, m)
	}
	return out
}
