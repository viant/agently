package sdk

import (
	"encoding/json"
	"strings"

	conv "github.com/viant/agently/client/conversation"
)

// IsElicitationMessage reports whether a message carries an elicitation id.
func IsElicitationMessage(msg *conv.Message) bool {
	if msg == nil || msg.ElicitationId == nil {
		return false
	}
	return strings.TrimSpace(*msg.ElicitationId) != ""
}

// IsElicitationPending reports whether a message is an active elicitation.
// Pending/open (or empty) status is considered active.
func IsElicitationPending(msg *conv.Message) bool {
	if !IsElicitationMessage(msg) {
		return false
	}
	if msg.Status == nil {
		return true
	}
	status := strings.ToLower(strings.TrimSpace(*msg.Status))
	return status == "" || status == "pending" || status == "open"
}

// ElicitationFromMessage builds an Elicitation snapshot from a message.
func ElicitationFromMessage(conversationID string, msg *conv.Message) *Elicitation {
	if !IsElicitationMessage(msg) {
		return nil
	}
	out := &Elicitation{
		ConversationID: conversationID,
		MessageID:      msg.Id,
		Role:           msg.Role,
		CreatedAt:      msg.CreatedAt,
	}
	if out.ConversationID == "" {
		out.ConversationID = msg.ConversationId
	}
	if msg.ElicitationId != nil {
		out.ElicitationID = strings.TrimSpace(*msg.ElicitationId)
	}
	if msg.Status != nil {
		out.Status = strings.TrimSpace(*msg.Status)
	}
	if msg.Content != nil {
		out.Content = *msg.Content
		if raw := strings.TrimSpace(out.Content); raw != "" {
			var m map[string]interface{}
			if err := json.Unmarshal([]byte(raw), &m); err == nil {
				out.Request = m
			}
		}
	}
	return out
}

// ElicitationFromEvent returns a parsed elicitation from a stream event when present.
func ElicitationFromEvent(ev *StreamEventEnvelope) *Elicitation {
	if ev == nil || ev.Message == nil {
		return nil
	}
	return ElicitationFromMessage(ev.ConversationID, ev.Message)
}
