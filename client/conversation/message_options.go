package conversation

import (
	"time"
)

// MessageOption configures a MutableMessage prior to persistence.
type MessageOption func(m *MutableMessage)

// Core identifiers and linkage
func WithId(id string) MessageOption { return func(m *MutableMessage) { m.SetId(id) } }
func WithConversationID(id string) MessageOption {
	return func(m *MutableMessage) { m.SetConversationID(id) }
}
func WithTurnID(id string) MessageOption { return func(m *MutableMessage) { m.SetTurnID(id) } }
func WithParentMessageID(id string) MessageOption {
	return func(m *MutableMessage) { m.SetParentMessageID(id) }
}
func WithLinkedConversationID(id string) MessageOption {
	return func(m *MutableMessage) { m.SetLinkedConversationID(id) }
}

// Metadata and attribution
func WithCreatedAt(t time.Time) MessageOption { return func(m *MutableMessage) { m.SetCreatedAt(t) } }
func WithUpdatedAt(t time.Time) MessageOption { return func(m *MutableMessage) { m.SetUpdatedAt(t) } }
func WithCreatedByUserID(id string) MessageOption {
	return func(m *MutableMessage) { m.SetCreatedByUserID(id) }
}
func WithArchived(v int) MessageOption { return func(m *MutableMessage) { m.SetArchived(v) } }
func WithSequence(v int) MessageOption { return func(m *MutableMessage) { m.SetSequence(v) } }

// Message content and semantics
func WithRole(role string) MessageOption     { return func(m *MutableMessage) { m.SetRole(role) } }
func WithStatus(status string) MessageOption { return func(m *MutableMessage) { m.SetStatus(status) } }
func WithType(typ string) MessageOption      { return func(m *MutableMessage) { m.SetType(typ) } }
func WithContent(content string) MessageOption {
	return func(m *MutableMessage) { m.SetContent(content) }
}
func WithInterim(v int) MessageOption        { return func(m *MutableMessage) { m.SetInterim(v) } }
func WithMode(mode string) MessageOption     { return func(m *MutableMessage) { m.SetMode(mode) } }
func WithToolName(name string) MessageOption { return func(m *MutableMessage) { m.SetToolName(name) } }
func WithElicitationID(id string) MessageOption {
	return func(m *MutableMessage) { m.SetElicitationID(id) }
}
func WithAttachmentPayloadID(id string) MessageOption {
	return func(m *MutableMessage) { m.SetAttachmentPayloadID(id) }
}
func WithElicitationPayloadID(id string) MessageOption {
	return func(m *MutableMessage) { m.SetElicitationPayloadID(id) }
}

// Optional summaries/tags (set via raw fields when no setter exists)
func WithContextSummary(s string) MessageOption {
	return func(m *MutableMessage) {
		m.ContextSummary = &s
		if m.Has != nil {
			m.Has.ContextSummary = true
		}
	}
}
func WithTags(s string) MessageOption {
	return func(m *MutableMessage) {
		m.Tags = &s
		if m.Has != nil {
			m.Has.Tags = true
		}
	}
}
func WithSupersededBy(id string) MessageOption {
	return func(m *MutableMessage) {
		m.SupersededBy = &id
		if m.Has != nil {
			m.Has.SupersededBy = true
		}
	}
}
