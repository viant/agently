package write

import "time"

var PackageName = "scheduler/schedule/write"

type Schedule struct {
	Id              string       `sqlx:"id,primaryKey" validate:"required"`
	Name            string       `sqlx:"name" validate:"required"`
	Description     *string      `sqlx:"description" json:",omitempty"`
	AgentRef        string       `sqlx:"agent_ref" validate:"required"`
	ModelOverride   *string      `sqlx:"model_override" json:",omitempty"`
	UserCredURL     *string      `sqlx:"user_cred_url" json:",omitempty"`
	Enabled         bool         `sqlx:"enabled" `
	StartAt         *time.Time   `sqlx:"start_at" json:",omitempty"`
	EndAt           *time.Time   `sqlx:"end_at" json:",omitempty"`
	ScheduleType    string       `sqlx:"schedule_type" validate:"required"`
	CronExpr        *string      `sqlx:"cron_expr" json:",omitempty"`
	IntervalSeconds *int         `sqlx:"interval_seconds" json:",omitempty"`
	Timezone        string       `sqlx:"timezone" validate:"required"`
	TimeoutSeconds  int          `sqlx:"timeout_seconds"`
	TaskPromptUri   *string      `sqlx:"task_prompt_uri" json:",omitempty"`
	TaskPrompt      *string      `sqlx:"task_prompt" json:",omitempty"`
	NextRunAt       *time.Time   `sqlx:"next_run_at" json:",omitempty"`
	LastRunAt       *time.Time   `sqlx:"last_run_at" json:",omitempty"`
	LastStatus      *string      `sqlx:"last_status" json:",omitempty"`
	LastError       *string      `sqlx:"last_error" json:",omitempty"`
	LeaseOwner      *string      `sqlx:"lease_owner" json:",omitempty"`
	LeaseUntil      *time.Time   `sqlx:"lease_until" json:",omitempty"`
	CreatedAt       *time.Time   `sqlx:"created_at" json:",omitempty"`
	UpdatedAt       *time.Time   `sqlx:"updated_at" json:",omitempty"`
	Has             *ScheduleHas `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

// Schedules is a helper slice type referenced by datly `dataType` tags
// to drive structql markers for the Schedules collection in Input.
type Schedules []Schedule

type ScheduleHas struct {
	Id              bool
	Name            bool
	Description     bool
	AgentRef        bool
	ModelOverride   bool
	UserCredURL     bool
	Enabled         bool
	StartAt         bool
	EndAt           bool
	ScheduleType    bool
	CronExpr        bool
	IntervalSeconds bool
	Timezone        bool
	TimeoutSeconds  bool
	TaskPromptUri   bool
	TaskPrompt      bool
	NextRunAt       bool
	LastRunAt       bool
	LastStatus      bool
	LastError       bool
	LeaseOwner      bool
	LeaseUntil      bool
	CreatedAt       bool
	UpdatedAt       bool
}

func (m *Schedule) ensureHas() {
	if m.Has == nil {
		m.Has = &ScheduleHas{}
	}
}
func (m *Schedule) SetId(v string)   { m.Id = v; m.ensureHas(); m.Has.Id = true }
func (m *Schedule) SetName(v string) { m.Name = v; m.ensureHas(); m.Has.Name = true }
func (m *Schedule) SetDescription(v string) {
	m.Description = &v
	m.ensureHas()
	m.Has.Description = true
}
func (m *Schedule) SetAgentRef(v string) { m.AgentRef = v; m.ensureHas(); m.Has.AgentRef = true }
func (m *Schedule) SetModelOverride(v string) {
	m.ModelOverride = &v
	m.ensureHas()
	m.Has.ModelOverride = true
}
func (m *Schedule) SetUserCredURL(v string) {
	m.UserCredURL = &v
	m.ensureHas()
	m.Has.UserCredURL = true
}
func (m *Schedule) SetEnabled(v bool)      { m.Enabled = v; m.ensureHas(); m.Has.Enabled = true }
func (m *Schedule) SetStartAt(v time.Time) { m.StartAt = &v; m.ensureHas(); m.Has.StartAt = true }
func (m *Schedule) SetEndAt(v time.Time)   { m.EndAt = &v; m.ensureHas(); m.Has.EndAt = true }
func (m *Schedule) SetScheduleType(v string) {
	m.ScheduleType = v
	m.ensureHas()
	m.Has.ScheduleType = true
}
func (m *Schedule) SetCronExpr(v string) { m.CronExpr = &v; m.ensureHas(); m.Has.CronExpr = true }
func (m *Schedule) SetIntervalSeconds(v int) {
	m.IntervalSeconds = &v
	m.ensureHas()
	m.Has.IntervalSeconds = true
}
func (m *Schedule) SetTimezone(v string) { m.Timezone = v; m.ensureHas(); m.Has.Timezone = true }
func (m *Schedule) SetTimeoutSeconds(v int) {
	m.TimeoutSeconds = v
	m.ensureHas()
	m.Has.TimeoutSeconds = true
}
func (m *Schedule) SetTaskPromptUri(v string) {
	m.TaskPromptUri = &v
	m.ensureHas()
	m.Has.TaskPromptUri = true
}
func (m *Schedule) SetTaskPrompt(v string)   { m.TaskPrompt = &v; m.ensureHas(); m.Has.TaskPrompt = true }
func (m *Schedule) SetNextRunAt(v time.Time) { m.NextRunAt = &v; m.ensureHas(); m.Has.NextRunAt = true }
func (m *Schedule) SetLastRunAt(v time.Time) { m.LastRunAt = &v; m.ensureHas(); m.Has.LastRunAt = true }
func (m *Schedule) SetLastStatus(v string)   { m.LastStatus = &v; m.ensureHas(); m.Has.LastStatus = true }
func (m *Schedule) SetLastError(v string)    { m.LastError = &v; m.ensureHas(); m.Has.LastError = true }
func (m *Schedule) SetLeaseOwner(v string)   { m.LeaseOwner = &v; m.ensureHas(); m.Has.LeaseOwner = true }
func (m *Schedule) SetLeaseUntil(v time.Time) {
	m.LeaseUntil = &v
	m.ensureHas()
	m.Has.LeaseUntil = true
}
func (m *Schedule) SetCreatedAt(v time.Time) { m.CreatedAt = &v; m.ensureHas(); m.Has.CreatedAt = true }
func (m *Schedule) SetUpdatedAt(v time.Time) { m.UpdatedAt = &v; m.ensureHas(); m.Has.UpdatedAt = true }
