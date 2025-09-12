package shared

import (
	"sort"

	read "github.com/viant/agently/internal/dao/message/read"
)

// BuildTranscript normalizes a transcript view from message rows.
func BuildTranscript(rows []*read.MessageView, excludeInterim bool) []*read.MessageView {
	if len(rows) == 0 {
		return rows
	}

	// Order by sequence if present, otherwise created_at
	sort.SliceStable(rows, func(i, j int) bool {
		li, lj := rows[i], rows[j]
		if li == nil || lj == nil {
			return i < j
		}
		if li.Sequence != nil && lj.Sequence != nil {
			return *li.Sequence < *lj.Sequence
		}
		if li.CreatedAt != nil && lj.CreatedAt != nil {
			return li.CreatedAt.Before(*lj.CreatedAt)
		}
		return i < j
	})

	// Compute latest attempts for tool messages
	latestAttempt := map[string]int{}
	for _, m := range rows {
		if m == nil || m.ToolCall == nil || m.ToolCall.OpID == "" {
			continue
		}
		key := m.ToolCall.OpID
		if m.ToolCall.RequestHash != nil && *m.ToolCall.RequestHash != "" {
			key = key + "|" + *m.ToolCall.RequestHash
		}
		if m.ToolCall.Attempt > latestAttempt[key] {
			latestAttempt[key] = m.ToolCall.Attempt
		}
	}

	// Filter, exclude control and interim, dedup tools
	var result []*read.MessageView
	for _, m := range rows {
		if m == nil {
			continue
		}
		if m.Type == "control" {
			continue
		}
		if excludeInterim && m.Interim != nil && *m.Interim == 1 {
			continue
		}
		switch m.Role {
		case "tool":
			if m.ToolCall != nil && m.ToolCall.OpID != "" {
				key := m.ToolCall.OpID
				if m.ToolCall.RequestHash != nil && *m.ToolCall.RequestHash != "" {
					key = key + "|" + *m.ToolCall.RequestHash
				}
				if latestAttempt[key] != m.ToolCall.Attempt {
					continue
				}
			}
			result = append(result, m)
		default:
			result = append(result, m)
			// ignore other roles by default
		}
	}
	return result
}
