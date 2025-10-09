package conversation

import (
	"path"
	"strings"
	"unsafe"

	"github.com/viant/agently/genai/prompt"
	"github.com/viant/agently/pkg/agently/conversation"
)

func (t *Turn) GetMessages() Messages {
	return *(*Messages)(unsafe.Pointer(&t.Message))
}

func (t *Turn) SetMessages(msg Messages) {
	t.Message = *(*[]*conversation.MessageView)(unsafe.Pointer(&msg))
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

func (t *Transcript) History(minimal bool) []*prompt.Message {

	transcript := *t
	if minimal {
		transcript = transcript[len(transcript)-1:]
	}

	normalized := transcript.Filter(func(v *Message) bool {
		if v == nil || v.IsArchived() || v.IsInterim() || v.Content == nil || *v.Content == "" {
			return false
		}
		// Only include regular chat text; exclude elicitation/status/tool/etc.
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
		// Collect attachments associated to this base message (joined via parent_message_id)
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
				attachments = append(attachments, &prompt.Attachment{
					Name: name,
					URI: func() string {
						if av.Uri != nil {
							return *av.Uri
						}
						return ""
					}(),
					Mime: av.MimeType,
					Data: data,
				})
			}

		}
		result = append(result, &prompt.Message{Role: role, Content: content, Attachment: attachments})
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
