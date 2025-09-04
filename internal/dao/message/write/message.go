package write

import "time"

var PackageName = "message/write"

type Message struct {
	Id               string      `sqlx:"id,primaryKey" validate:"required"`
	ConversationID   string      `sqlx:"conversation_id" validate:"required"`
	TurnID           *string     `sqlx:"turn_id" json:",omitempty"`
	Sequence         *int        `sqlx:"sequence" json:",omitempty"`
	CreatedAt        *time.Time  `sqlx:"created_at" json:",omitempty"`
	Role             string      `sqlx:"role" validate:"required"`
	Type             string      `sqlx:"type" validate:"required"`
	Content          string      `sqlx:"content" validate:"required"`
	ContextSummary   *string     `sqlx:"context_summary" json:",omitempty"`
	Tags             *string     `sqlx:"tags" json:",omitempty"`
	Interim          *int        `sqlx:"interim" json:",omitempty"`
	ElicitationID    *string     `sqlx:"elicitation_id" json:",omitempty"`
	ParentMessageID  *string     `sqlx:"parent_message_id" json:",omitempty"`
	ModelCallPresent *int        `sqlx:"model_call_present" json:",omitempty"`
	ToolCallPresent  *int        `sqlx:"tool_call_present" json:",omitempty"`
	SupersededBy     *string     `sqlx:"superseded_by" json:",omitempty"`
	ToolName         *string     `sqlx:"tool_name" json:",omitempty"`
	Has              *MessageHas `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

type MessageHas struct {
	Id               bool
	ConversationID   bool
	TurnID           bool
	Sequence         bool
	CreatedAt        bool
	Role             bool
	Type             bool
	Content          bool
	ContextSummary   bool
	Tags             bool
	Interim          bool
	ElicitationID    bool
	ParentMessageID  bool
	ModelCallPresent bool
	ToolCallPresent  bool
	SupersededBy     bool
	ToolName         bool
}

func (m *Message) ensureHas() {
	if m.Has == nil {
		m.Has = &MessageHas{}
	}
}
func (m *Message) SetId(v string) { m.Id = v; m.ensureHas(); m.Has.Id = true }
func (m *Message) SetConversationID(v string) {
	m.ConversationID = v
	m.ensureHas()
	m.Has.ConversationID = true
}
func (m *Message) SetTurnID(v string)       { m.TurnID = &v; m.ensureHas(); m.Has.TurnID = true }
func (m *Message) SetSequence(v int)        { m.Sequence = &v; m.ensureHas(); m.Has.Sequence = true }
func (m *Message) SetCreatedAt(v time.Time) { m.CreatedAt = &v; m.ensureHas(); m.Has.CreatedAt = true }
func (m *Message) SetRole(v string)         { m.Role = v; m.ensureHas(); m.Has.Role = true }
func (m *Message) SetType(v string)         { m.Type = v; m.ensureHas(); m.Has.Type = true }
func (m *Message) SetContent(v string)      { m.Content = v; m.ensureHas(); m.Has.Content = true }
func (m *Message) SetToolName(v string)     { m.ToolName = &v; m.ensureHas(); m.Has.ToolName = true }
func (m *Message) SetParentMessageID(v string) {
	m.ParentMessageID = &v
	m.ensureHas()
	m.Has.ParentMessageID = true
}

func (m *Message) SetElicitationID(id string) {
	m.ElicitationID = &id
	m.ensureHas()
	m.Has.ElicitationID = true
}
