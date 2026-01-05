package write

import "time"

var PackageName = "turn/write"

// Turn mirrors the turn table for upsert operations.
type Turn struct {
	Id                    string     `sqlx:"id,primaryKey" validate:"required"`
	ConversationID        string     `sqlx:"conversation_id" validate:"required"`
	CreatedAt             *time.Time `sqlx:"created_at" json:",omitempty"`
	QueueSeq              *int64     `sqlx:"queue_seq" json:",omitempty"`
	Status                string     `sqlx:"status" validate:"required"`
	StartedByMessageID    *string    `sqlx:"started_by_message_id" json:",omitempty"`
	RetryOf               *string    `sqlx:"retry_of" json:",omitempty"`
	AgentIDUsed           *string    `sqlx:"agent_id_used" json:",omitempty"`
	AgentConfigUsedID     *string    `sqlx:"agent_config_used_id" json:",omitempty"`
	ModelOverrideProvider *string    `sqlx:"model_override_provider" json:",omitempty"`
	ModelOverride         *string    `sqlx:"model_override" json:",omitempty"`
	ModelParamsOverride   *string    `sqlx:"model_params_override" json:",omitempty"`
	ErrorMessage          *string    `sqlx:"error_message" json:",omitempty"`
	Has                   *TurnHas   `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

type TurnHas struct {
	Id                    bool
	ConversationID        bool
	CreatedAt             bool
	QueueSeq              bool
	Status                bool
	StartedByMessageID    bool
	RetryOf               bool
	AgentIDUsed           bool
	AgentConfigUsedID     bool
	ModelOverrideProvider bool
	ModelOverride         bool
	ModelParamsOverride   bool
	ErrorMessage          bool
}

func (t *Turn) SetId(v string) { t.Id = v; ensureHas(&t.Has); t.Has.Id = true }
func (t *Turn) SetConversationID(v string) {
	t.ConversationID = v
	ensureHas(&t.Has)
	t.Has.ConversationID = true
}
func (t *Turn) SetCreatedAt(v time.Time) { t.CreatedAt = &v; ensureHas(&t.Has); t.Has.CreatedAt = true }
func (t *Turn) SetQueueSeq(v int64)      { t.QueueSeq = &v; ensureHas(&t.Has); t.Has.QueueSeq = true }
func (t *Turn) SetStatus(v string)       { t.Status = v; ensureHas(&t.Has); t.Has.Status = true }
func (t *Turn) SetStartedByMessageID(v string) {
	t.StartedByMessageID = &v
	ensureHas(&t.Has)
	t.Has.StartedByMessageID = true
}
func (t *Turn) SetRetryOf(v string) { t.RetryOf = &v; ensureHas(&t.Has); t.Has.RetryOf = true }
func (t *Turn) SetAgentIDUsed(v string) {
	t.AgentIDUsed = &v
	ensureHas(&t.Has)
	t.Has.AgentIDUsed = true
}
func (t *Turn) SetAgentConfigUsedID(v string) {
	t.AgentConfigUsedID = &v
	ensureHas(&t.Has)
	t.Has.AgentConfigUsedID = true
}
func (t *Turn) SetModelOverrideProvider(v string) {
	t.ModelOverrideProvider = &v
	ensureHas(&t.Has)
	t.Has.ModelOverrideProvider = true
}
func (t *Turn) SetModelOverride(v string) {
	t.ModelOverride = &v
	ensureHas(&t.Has)
	t.Has.ModelOverride = true
}
func (t *Turn) SetModelParamsOverride(v string) {
	t.ModelParamsOverride = &v
	ensureHas(&t.Has)
	t.Has.ModelParamsOverride = true
}
func (t *Turn) SetErrorMessage(v string) {
	t.ErrorMessage = &v
	ensureHas(&t.Has)
	t.Has.ErrorMessage = true
}

type TurnSlice []*Turn
type IndexedTurn map[string]*Turn

func (s TurnSlice) IndexById() IndexedTurn {
	res := IndexedTurn{}
	for i, it := range s {
		if it != nil {
			res[it.Id] = s[i]
		}
	}
	return res
}

func ensureHas(h **TurnHas) {
	if *h == nil {
		*h = &TurnHas{}
	}
}
