package post

import (
	"time"
)

var PackageName = "conversation/post"

type Conversations struct {
	Conversations []*Conversation
}

type Conversation struct {
	Id                   string           `sqlx:"id,primaryKey" validate:"required"`
	Summary              *string          `sqlx:"summary" json:",omitempty"`
	AgentName            string           `sqlx:"agent_name" `
	CreatedAt            *time.Time       `sqlx:"created_at" json:",omitempty"`
	LastActivity         *time.Time       `sqlx:"last_activity" json:",omitempty"`
	UsageInputTokens     int              `sqlx:"usage_input_tokens" json:",omitempty"`
	UsageOutputTokens    int              `sqlx:"usage_output_tokens" json:",omitempty"`
	UsageEmbeddingTokens int              `sqlx:"usage_embedding_tokens" json:",omitempty"`
	Message              []*Message       `sqlx:"-" on:"Id:id=ConversationId:conversation_id" view:",table=message" sql:"uri=conversation/message.sql" `
	Has                  *ConversationHas `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
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
	Id                   bool
	Summary              bool
	AgentName            bool
	CreatedAt            bool
	LastActivity         bool
	UsageInputTokens     bool
	UsageOutputTokens    bool
	UsageEmbeddingTokens bool
	Message              bool
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

func (c *Conversation) SetUsageInputTokens(value int) {
	c.UsageInputTokens = value
	c.Has.UsageInputTokens = true
}

func (c *Conversation) SetUsageOutputTokens(value int) {
	c.UsageOutputTokens = value
	c.Has.UsageOutputTokens = true
}

func (c *Conversation) SetUsageEmbeddingTokens(value int) {
	c.UsageEmbeddingTokens = value
	c.Has.UsageEmbeddingTokens = true
}
