package post

import (
	"time"
)

var PackageName = "conversation/post"

type Conversations struct {
	Conversations []*Conversation
}

type Conversation struct {
	Id           string           `sqlx:"id,primaryKey" validate:"required"`
	Summary      *string          `sqlx:"summary" json:",omitempty"`
	AgentName    string           `sqlx:"agent_name" `
	CreatedAt    *time.Time       `sqlx:"created_at" json:",omitempty"`
	LastActivity *time.Time       `sqlx:"last_activity" json:",omitempty"`
	Message      []*Message       `sqlx:"-" on:"Id:id=ConversationId:conversation_id" view:",table=message" sql:"uri=conversation/message.sql" `
	Has          *ConversationHas `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

type Message struct {
	Id             string      `sqlx:"id,primaryKey" validate:"required"`
	ConversationId string      `sqlx:"conversation_id" `
	Role           string      `sqlx:"role" validate:"required"`
	Content        string      `sqlx:"content" validate:"required"`
	ToolName       *string     `sqlx:"tool_name" json:",omitempty"`
	CreatedAt      *time.Time  `sqlx:"created_at" json:",omitempty"`
	Has            *MessageHas `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

type MessageHas struct {
	Id             bool
	ConversationId bool
	Role           bool
	Content        bool
	ToolName       bool
	CreatedAt      bool
}

type ConversationHas struct {
	Id           bool
	Summary      bool
	AgentName    bool
	CreatedAt    bool
	LastActivity bool
	Message      bool
}

func (c *Conversation) SetId(value string) {
	c.Id = value
	c.Has.Id = true
}

func (c *Conversation) SetSummary(value string) {
	c.Summary = &value
	c.Has.Summary = true
}

func (c *Conversation) SetAgentName(value string) {
	c.AgentName = value
	c.Has.AgentName = true
}

func (c *Conversation) SetCreatedAt(value time.Time) {
	c.CreatedAt = &value
	c.Has.CreatedAt = true
}

func (c *Conversation) SetLastActivity(value time.Time) {
	c.LastActivity = &value
	c.Has.LastActivity = true
}
