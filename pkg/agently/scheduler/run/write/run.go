package write

import "time"

var PackageName = "scheduler/run/write"

type Run struct {
	Id                 string     `sqlx:"id,primaryKey" validate:"required"`
	ScheduleId         string     `sqlx:"schedule_id" validate:"required"`
	CreatedAt          *time.Time `sqlx:"created_at" json:",omitempty"`
	UpdatedAt          *time.Time `sqlx:"updated_at" json:",omitempty"`
	Status             string     `sqlx:"status" validate:"required"`
	ErrorMessage       *string    `sqlx:"error_message" json:",omitempty"`
	PreconditionRanAt  *time.Time `sqlx:"precondition_ran_at" json:",omitempty"`
	PreconditionPassed *int       `sqlx:"precondition_passed" json:",omitempty"`
	PreconditionResult *string    `sqlx:"precondition_result" json:",omitempty"`
	ConversationId     *string    `sqlx:"conversation_id" json:",omitempty"`
	ConversationKind   string     `sqlx:"conversation_kind" validate:"required"`
	StartedAt          *time.Time `sqlx:"started_at" json:",omitempty"`
	CompletedAt        *time.Time `sqlx:"completed_at" json:",omitempty"`
	Has                *RunHas    `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

// Runs is a helper slice type referenced by datly `dataType` tags
// to drive structql markers for the Runs collection in Input.
type Runs []Run

type RunHas struct {
	Id, ScheduleId, CreatedAt, UpdatedAt                      bool
	Status, ErrorMessage                                      bool
	PreconditionRanAt, PreconditionPassed, PreconditionResult bool
	ConversationId, ConversationKind                          bool
	StartedAt, CompletedAt                                    bool
}

func (m *Run) ensureHas() {
	if m.Has == nil {
		m.Has = &RunHas{}
	}
}
func (m *Run) SetId(v string)           { m.Id = v; m.ensureHas(); m.Has.Id = true }
func (m *Run) SetScheduleId(v string)   { m.ScheduleId = v; m.ensureHas(); m.Has.ScheduleId = true }
func (m *Run) SetCreatedAt(v time.Time) { m.CreatedAt = &v; m.ensureHas(); m.Has.CreatedAt = true }
func (m *Run) SetUpdatedAt(v time.Time) { m.UpdatedAt = &v; m.ensureHas(); m.Has.UpdatedAt = true }
func (m *Run) SetStatus(v string)       { m.Status = v; m.ensureHas(); m.Has.Status = true }
func (m *Run) SetErrorMessage(v string) {
	m.ErrorMessage = &v
	m.ensureHas()
	m.Has.ErrorMessage = true
}
func (m *Run) SetPreconditionRanAt(v time.Time) {
	m.PreconditionRanAt = &v
	m.ensureHas()
	m.Has.PreconditionRanAt = true
}
func (m *Run) SetPreconditionPassed(v int) {
	m.PreconditionPassed = &v
	m.ensureHas()
	m.Has.PreconditionPassed = true
}
func (m *Run) SetPreconditionResult(v string) {
	m.PreconditionResult = &v
	m.ensureHas()
	m.Has.PreconditionResult = true
}
func (m *Run) SetConversationId(v string) {
	m.ConversationId = &v
	m.ensureHas()
	m.Has.ConversationId = true
}
func (m *Run) SetConversationKind(v string) {
	m.ConversationKind = v
	m.ensureHas()
	m.Has.ConversationKind = true
}
func (m *Run) SetStartedAt(v time.Time) { m.StartedAt = &v; m.ensureHas(); m.Has.StartedAt = true }
func (m *Run) SetCompletedAt(v time.Time) {
	m.CompletedAt = &v
	m.ensureHas()
	m.Has.CompletedAt = true
}
