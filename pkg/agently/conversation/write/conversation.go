package write

import (
	"time"
)

var PackageName = "conversation/write"

type Conversations struct {
	Conversations []*Conversation
}

type Conversation struct {
	Id                       string     `sqlx:"id,primaryKey" validate:"required"`
	Summary                  *string    `sqlx:"summary" json:",omitempty"`
	AgentId                  string     `sqlx:"agent_id" `
	Title                    *string    `sqlx:"title" json:",omitempty"`
	ConversationParentId     string     `sqlx:"conversation_parent_id" `
	ConversationParentTurnId string     `sqlx:"conversation_parent_turn_id" `
	Visibility               *string    `sqlx:"visibility" json:",omitempty"`
	Shareable                int        `sqlx:"shareable" json:",omitempty"`
	CreatedAt                *time.Time `sqlx:"created_at" json:",omitempty"`
	UpdatedAt                *time.Time `sqlx:"updated_at" json:",omitempty"`
	LastActivity             *time.Time `sqlx:"last_activity" json:",omitempty"`
	UsageInputTokens         int        `sqlx:"usage_input_tokens" json:",omitempty"`
	UsageOutputTokens        int        `sqlx:"usage_output_tokens" json:",omitempty"`
	UsageEmbeddingTokens     int        `sqlx:"usage_embedding_tokens" json:",omitempty"`
	CreatedByUserID          *string    `sqlx:"created_by_user_id" json:",omitempty"`
	DefaultModelProvider     *string    `sqlx:"default_model_provider" json:",omitempty"`
	DefaultModel             *string    `sqlx:"default_model" json:",omitempty"`
	DefaultModelParams       *string    `sqlx:"default_model_params" json:",omitempty"`
	Metadata                 *string    `sqlx:"metadata" json:",omitempty"`
	// Scheduling annotations for discriminating scheduled conversations
	Scheduled        int     `sqlx:"scheduled" json:",omitempty"`
	ScheduleId       *string `sqlx:"schedule_id" json:",omitempty"`
	ScheduleRunId    *string `sqlx:"schedule_run_id" json:",omitempty"`
	ScheduleKind     *string `sqlx:"schedule_kind" json:",omitempty"`
	ScheduleTimezone *string `sqlx:"schedule_timezone" json:",omitempty"`
	ScheduleCronExpr *string `sqlx:"schedule_cron_expr" json:",omitempty"`
	// ExternalTaskRef links this conversation to an external A2A task reference
	ExternalTaskRef *string          `sqlx:"external_task_ref" json:",omitempty"`
	Status          *string          `sqlx:"status" json:",omitempty"`
	Has             *ConversationHas `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

type ConversationHas struct {
	Id                       bool
	Summary                  bool
	AgentId                  bool
	ConversationParentId     bool
	ConversationParentTurnId bool
	Title                    bool
	Visibility               bool
	Shareable                bool
	CreatedAt                bool
	UpdatedAt                bool
	LastActivity             bool
	UsageInputTokens         bool
	UsageOutputTokens        bool
	UsageEmbeddingTokens     bool
	CreatedByUserID          bool
	DefaultModelProvider     bool
	DefaultModel             bool
	DefaultModelParams       bool
	Metadata                 bool
	Scheduled                bool
	ScheduleId               bool
	ScheduleRunId            bool
	ScheduleKind             bool
	ScheduleTimezone         bool
	ScheduleCronExpr         bool
	ExternalTaskRef          bool
	Status                   bool
}

func NewConversationStatus(id, status string) *Conversation {
	ret := &Conversation{Has: &ConversationHas{}}
	ret.SetStatus(status)
	ret.SetId(id)
	ret.SetUpdatedAt(time.Now())
	return ret
}

func (c *Conversation) SetId(value string) {
	c.Id = value
	c.Has.Id = true
}

func (c *Conversation) SetStatus(value string) {
	c.Status = &value
	c.Has.Status = true
}

func (c *Conversation) SetSummary(value string) {
	c.Summary = &value
	c.Has.Summary = true
}

func (c *Conversation) SetAgentId(value string) {
	c.AgentId = value
	c.Has.AgentId = true
}

func (c *Conversation) SetConversationParentId(value string) {
	c.ConversationParentId = value
	c.Has.ConversationParentId = true
}

func (c *Conversation) SetConversationParentTurnId(value string) {
	c.ConversationParentTurnId = value
	c.Has.ConversationParentTurnId = true
}

func (c *Conversation) SetTitle(value string) {
	c.Title = &value
	c.Has.Title = true
}

func (c *Conversation) SetVisibility(value string) {
	c.Visibility = &value
	c.Has.Visibility = true
}

func (c *Conversation) SetShareable(value bool) {
	if value {
		c.Shareable = 1
	} else {
		c.Shareable = 0
	}
	c.Has.Shareable = true
}

func (c *Conversation) SetCreatedAt(value time.Time) {
	c.CreatedAt = &value
	c.Has.CreatedAt = true
}

func (c *Conversation) SetUpdatedAt(value time.Time) {
	c.UpdatedAt = &value
	c.Has.UpdatedAt = true
}

func (c *Conversation) SetCreatedByUserID(value string) {
	c.CreatedByUserID = &value
	c.Has.CreatedByUserID = true
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

func (c *Conversation) SetDefaultModelProvider(value string) {
	c.DefaultModelProvider = &value
	c.Has.DefaultModelProvider = true
}

func (c *Conversation) SetDefaultModel(value string) {
	c.DefaultModel = &value
	c.Has.DefaultModel = true
}

func (c *Conversation) SetDefaultModelParams(value string) {
	c.DefaultModelParams = &value
	c.Has.DefaultModelParams = true
}

func (c *Conversation) SetMetadata(value string) {
	c.Metadata = &value
	c.Has.Metadata = true
}

func (c *Conversation) SetScheduled(value int) {
	c.Scheduled = value
	c.Has.Scheduled = true
}

func (c *Conversation) SetScheduleId(value string) {
	c.ScheduleId = &value
	c.Has.ScheduleId = true
}

func (c *Conversation) SetScheduleRunId(value string) {
	c.ScheduleRunId = &value
	c.Has.ScheduleRunId = true
}

func (c *Conversation) SetScheduleKind(value string) {
	c.ScheduleKind = &value
	c.Has.ScheduleKind = true
}

func (c *Conversation) SetScheduleTimezone(value string) {
	c.ScheduleTimezone = &value
	c.Has.ScheduleTimezone = true
}

func (c *Conversation) SetScheduleCronExpr(value string) {
	c.ScheduleCronExpr = &value
	c.Has.ScheduleCronExpr = true
}

func (c *Conversation) SetExternalTaskRef(value string) {
	c.ExternalTaskRef = &value
	if c.Has == nil {
		c.Has = &ConversationHas{}
	}
	c.Has.ExternalTaskRef = true
}
