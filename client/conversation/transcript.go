package conversation

import (
	"fmt"
	"path"
	"strings"
	"unsafe"

	"github.com/viant/agently/genai/llm"
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
		if v == nil || v.IsInterim() || v.Content == nil || *v.Content == "" {
			return false
		}
		// Only include regular chat text; exclude elicitation/status/tool/etc.
		if strings.ToLower(strings.TrimSpace(v.Type)) != "text" {
			return false
		}
		role := strings.ToLower(strings.TrimSpace(v.Role))
		return role == "user" || role == "assistant"
	})

	if n := len(normalized); n > 0 && query != "" {
		last := normalized[n-1]
		if last != nil && last.Content != nil && strings.EqualFold(strings.TrimSpace(last.Role), string(llm.RoleUser)) &&
			strings.TrimSpace(*last.Content) == strings.TrimSpace(query) {
			normalized = normalized[:n-1]
		}
	}

	var result []*prompt.Message
	for _, v := range normalized {

		role := v.Role
		content := ""
		if v.Content != nil {
			content = *v.Content
		}
		if v.Elicitation != nil {
			role = llm.RoleUser.String()
			userData := ""
			if v.Elicitation.InlineBody != nil {
				userData = string(*v.Elicitation.InlineBody)
			}
			if userData == "" {
				userData = fmt.Sprintf("elicitation status: %v", v.Status)
			}
			content = userData
		}
		// Collect attachments associated to this base message (joined via parent_message_id)
		var atts []*prompt.Attachment
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
				atts = append(atts, &prompt.Attachment{
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
		result = append(result, &prompt.Message{Role: role, Content: content, Attachment: atts})
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
