package conversation

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

func (m *Message) IsInterim() bool {
	if m != nil && m.Interim == 1 {
		return true
	}
	return false
}

func (m *Message) IsArchived() bool {
	if m == nil {
		return false
	}
	return m.Archived != nil && *m.Archived == 1
}

// GetContent returns the printable content for this message.
// - For tool-call messages, it prefers the response payload inline body.
// - For user/assistant messages, it returns the message content field.
func (m *Message) GetContent() string {
	if m == nil {
		return ""
	}
	if m.ToolCall != nil && m.ToolCall.ResponsePayload != nil && m.ToolCall.ResponsePayload.InlineBody != nil {
		return *m.ToolCall.ResponsePayload.InlineBody
	}
	if m.RawContent != nil && strings.TrimSpace(*m.RawContent) != "" {
		return *m.RawContent
	}
	if m.Content != nil {
		return *m.Content
	}
	return ""
}

func (m *Message) GetContentPreferContent() string {
	if m == nil {
		return ""
	}
	if m.ToolCall != nil && m.ToolCall.ResponsePayload != nil && m.ToolCall.ResponsePayload.InlineBody != nil {
		return *m.ToolCall.ResponsePayload.InlineBody
	}
	if m.Content != nil && strings.TrimSpace(*m.Content) != "" {
		return *m.Content
	}
	if m.RawContent != nil {
		return *m.RawContent
	}
	return ""
}

// ToolCallArguments returns parsed arguments for a tool-call message.
// It prefers the request payload inline JSON body when present. When parsing
// fails or no payload is present, it returns an empty map.
func (m *Message) ToolCallArguments() map[string]interface{} {
	args := map[string]interface{}{}
	if m == nil || m.ToolCall == nil || m.ToolCall.RequestPayload == nil || m.ToolCall.RequestPayload.InlineBody == nil {
		return args
	}
	raw := strings.TrimSpace(*m.ToolCall.RequestPayload.InlineBody)
	if raw == "" {
		return args
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
		args = parsed
	}
	return args
}

type Messages []*Message

type IndexedMessages map[string]*Message

// BuildMatchIndex returns a set of tool-call opIds that should be
// included for a continuation anchored at anchorID/anchorTime.
func (n IndexedMessages) BuildMatchIndex(anchorID string, anchorTime time.Time) map[string]bool {
	out := map[string]bool{}
	for opID, tmsg := range n {
		if tmsg == nil {
			continue
		}
		if tmsg.ToolCall != nil && tmsg.ToolCall.TraceId != nil {
			if matchByID := strings.TrimSpace(*tmsg.ToolCall.TraceId) == strings.TrimSpace(anchorID); matchByID {
				out[opID] = true
			}
			continue
		}

		matchByTime := tmsg.CreatedAt.After(anchorTime)
		if matchByTime && tmsg.Content != nil {
			out[*tmsg.Content] = true
		}
	}
	return out
}

// LatestByCreatedAt returns the last non-nil message by CreatedAt timestamp.
// When messages are empty or all nil, it returns nil.
func (m Messages) LatestByCreatedAt() *Message {
	if len(m) == 0 {
		return nil
	}
	var latest *Message
	for _, v := range m {
		if v == nil {
			continue
		}
		if latest == nil || v.CreatedAt.After(latest.CreatedAt) {
			latest = v
		}
	}
	return latest
}

// SortByCreatedAt sorts the messages in-place by CreatedAt.
// When asc is true, earlier messages come first; otherwise latest first.
func (m Messages) SortByCreatedAt(asc bool) {
	sort.SliceStable(m, func(i, j int) bool {
		if m[i] == nil || m[j] == nil {
			return false
		}
		if asc {
			return m[i].CreatedAt.Before(m[j].CreatedAt)
		}
		return m[i].CreatedAt.After(m[j].CreatedAt)
	})
}

// SortedByCreatedAt returns a new slice with messages ordered by CreatedAt.
// When asc is true, earlier messages come first; otherwise latest first.
func (m Messages) SortedByCreatedAt(asc bool) Messages {
	out := make(Messages, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	out.SortByCreatedAt(asc)
	return out
}
