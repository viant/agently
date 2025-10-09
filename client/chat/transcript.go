package chat

import (
	"path"
	"sort"
	"strings"
	"unsafe"

	"github.com/viant/agently/genai/prompt"
)

func (c *Conversation) GetTranscript() Transcript {
	if c.Transcript == nil {
		return nil
	}
	return *(*Transcript)(unsafe.Pointer(&c.Transcript))
}

func (t *Turn) GetMessages() Messages {
	return *(*Messages)(unsafe.Pointer(&t.Message))
}

func (m *Message) IsInterim() bool { return m != nil && m.Interim == 1 }

func (m *Message) IsArchived() bool {
	if m == nil {
		return false
	}
	return m.Archived != nil && *m.Archived == 1
}

type Messages []*Message

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

func (m Messages) SortedByCreatedAt(asc bool) Messages {
	out := make(Messages, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	out.SortByCreatedAt(asc)
	return out
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

func (t *Transcript) History(minimal bool) []*prompt.Message {
	transcript := *t
	if minimal {
		transcript = transcript[len(transcript)-1:]
	}
	normalized := transcript.Filter(func(v *Message) bool {
		if v == nil || v.IsArchived() || v.IsInterim() || v.Content == nil || *v.Content == "" {
			return false
		}
		if strings.ToLower(strings.TrimSpace(v.Type)) != "text" {
			return false
		}
		role := strings.ToLower(strings.TrimSpace(v.Role))
		return role == "user" || role == "assistant"
	})
	var result []*prompt.Message
	for _, v := range normalized {
		role := v.Role
		content := ""
		if v.Content != nil {
			content = *v.Content
		}
		var attachments []*prompt.Attachment
		if v.Attachment != nil && len(v.Attachment) > 0 {
			for _, av := range v.Attachment {
				if av == nil {
					continue
				}
				var data []byte
				if av.InlineBody != nil {
					data = []byte(*av.InlineBody)
				}
				name := ""
				if av.Uri != nil && *av.Uri != "" {
					name = path.Base(*av.Uri)
				}
				attachments = append(attachments, &prompt.Attachment{Name: name, URI: func() string {
					if av.Uri != nil {
						return *av.Uri
					}
					return ""
				}(), Mime: av.MimeType, Data: data})
			}
		}
		result = append(result, &prompt.Message{Role: role, Content: content, Attachment: attachments})
	}
	return result
}

func (t Transcript) UniqueToolNames() []string {
	if len(t) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	for _, turn := range t {
		if turn == nil || len(turn.Message) == 0 {
			continue
		}
		for _, m := range turn.Message {
			if m == nil || m.ToolCall == nil {
				continue
			}
			name := strings.TrimSpace(m.ToolCall.ToolName)
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			out = append(out, name)
		}
	}
	return out
}
