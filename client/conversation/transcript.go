package conversation

import (
	"strings"
	"unsafe"

	"github.com/viant/agently/genai/prompt"
)

func (t *Turn) GetMessages() Messages {
	return *(*Messages)(unsafe.Pointer(&t.Message))
}

func (t *Turn) ToolCalls() Messages {
	filtered := t.Filter(func(v *Message) bool {
		if v.ToolCall != nil {
			return true
		}
		return false
	})
	return filtered
}

func (t *Transcript) History(query string) []*prompt.Message {
	normalized := t.Filter(func(v *Message) bool {
		if v == nil || v.Type == "control" || v.IsInterim() || v.Content == nil || *v.Content == "" {
			return false
		}
		role := strings.ToLower(strings.TrimSpace(v.Role))
		return role == "user" || role == "assistant"
	})

	if n := len(normalized); n > 0 && query != "" {
		last := normalized[n-1]
		if last != nil && last.Content != nil && strings.EqualFold(strings.TrimSpace(last.Role), "user") &&
			strings.TrimSpace(*last.Content) == strings.TrimSpace(query) {
			normalized = normalized[:n-1]
		}
	}
	normalized.SortedByCreatedAt(true)

	var result []*prompt.Message
	for _, v := range normalized {
		content := ""
		if v.Content != nil {
			content = *v.Content
		}
		result = append(result, &prompt.Message{Role: v.Role, Content: content})
	}
	return result
}

func (t *Turn) Filter(f func(v *Message) bool) Messages {
	result := make(Messages, 0)
	for _, m := range t.GetMessages() {
		if f(m) {
			result = append(result, m)
		}
	}
	return result
}

func (t *Transcript) Filter(f func(v *Message) bool) Messages {
	var result Messages
	for _, turn := range *t {
		for _, message := range turn.GetMessages() {
			if f(message) {
				result = append(result, message)
			}
		}
	}
	return result
}
