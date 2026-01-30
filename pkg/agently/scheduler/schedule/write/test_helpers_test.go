package write

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/viant/agently/internal/testutil/dbtest"
	"github.com/viant/xdatly/handler"
	"github.com/viant/xdatly/handler/auth"
	"github.com/viant/xdatly/handler/differ"
	hhttp "github.com/viant/xdatly/handler/http"
	"github.com/viant/xdatly/handler/logger"
	"github.com/viant/xdatly/handler/mbus"
	"github.com/viant/xdatly/handler/sqlx"
	"github.com/viant/xdatly/handler/state"
	"github.com/viant/xdatly/handler/validator"
)

type scheduleSession struct {
	input     *Input
	sqlx      *sqlx.Service
	validator *validator.Service
}

func newScheduleSession(input *Input, db *sql.DB, v *fakeValidator) *scheduleSession {
	return &scheduleSession{
		input:     input,
		sqlx:      sqlx.New(&sqliteScheduleSQLX{db: db}),
		validator: validator.New(v),
	}
}

func (s *scheduleSession) Validator() *validator.Service { return s.validator }
func (s *scheduleSession) Differ() *differ.Service       { return nil }
func (s *scheduleSession) MessageBus() *mbus.Service     { return nil }
func (s *scheduleSession) Db(opts ...sqlx.Option) (*sqlx.Service, error) {
	return s.sqlx, nil
}
func (s *scheduleSession) Stater() *state.Service { return state.New(&stubInjector{input: s.input}) }
func (s *scheduleSession) FlushTemplate(context.Context) error {
	return nil
}
func (s *scheduleSession) Session(ctx context.Context, route *hhttp.Route, opts ...state.Option) (handler.Session, error) {
	return s, nil
}
func (s *scheduleSession) Http() hhttp.Http { return nil }
func (s *scheduleSession) Auth() auth.Auth  { return nil }
func (s *scheduleSession) Logger() logger.Logger {
	return nil
}

type stubInjector struct {
	input *Input
}

func (s *stubInjector) Into(ctx context.Context, any interface{}, opt ...state.Option) error {
	return s.Bind(ctx, any, opt...)
}

func (s *stubInjector) Bind(ctx context.Context, any interface{}, _ ...state.Option) error {
	switch dst := any.(type) {
	case *Input:
		*dst = *s.input
	}
	return nil
}

func (s *stubInjector) Value(context.Context, string) (interface{}, bool, error) {
	return nil, false, nil
}

func (s *stubInjector) ValuesOf(context.Context, interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}

type fakeValidator struct {
	violate bool
	err     error
}

func (v *fakeValidator) Validate(ctx context.Context, any interface{}, opts ...validator.Option) (*validator.Validation, error) {
	options := &validator.Options{}
	options.Apply(opts)
	if options.WithValidation == nil {
		options.WithValidation = validator.NewValidation()
	}
	if v.violate {
		options.WithValidation.Append(options.Location, "id", "dup", "unique", "")
	}
	return options.WithValidation, v.err
}

type sqliteScheduleSQLX struct {
	db *sql.DB
}

func (s *sqliteScheduleSQLX) Allocate(ctx context.Context, tableName string, dest interface{}, selector string) error {
	return nil
}
func (s *sqliteScheduleSQLX) Load(ctx context.Context, tableName string, data interface{}) error {
	return nil
}
func (s *sqliteScheduleSQLX) Flush(ctx context.Context, tableName string) error { return nil }
func (s *sqliteScheduleSQLX) Insert(tableName string, data interface{}) error {
	rec, ok := data.(*Schedule)
	if !ok {
		return errors.New("invalid insert data")
	}
	_, err := s.db.Exec(`
		INSERT INTO schedule (
			id, name, description, agent_ref, model_override, enabled,
			start_at, end_at, schedule_type, cron_expr, interval_seconds, timezone, timeout_seconds,
			task_prompt_uri, task_prompt, next_run_at, last_run_at, last_status, last_error,
			lease_owner, lease_until, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.Id, rec.Name, rec.Description, rec.AgentRef, rec.ModelOverride, rec.Enabled,
		rec.StartAt, rec.EndAt, rec.ScheduleType, rec.CronExpr, rec.IntervalSeconds, rec.Timezone, rec.TimeoutSeconds,
		rec.TaskPromptUri, rec.TaskPrompt, rec.NextRunAt, rec.LastRunAt, rec.LastStatus, rec.LastError,
		rec.LeaseOwner, rec.LeaseUntil, rec.CreatedAt, rec.UpdatedAt,
	)
	return err
}
func (s *sqliteScheduleSQLX) Update(tableName string, data interface{}) error {
	rec, ok := data.(*Schedule)
	if !ok {
		return errors.New("invalid update data")
	}
	_, err := s.db.Exec(`
		UPDATE schedule SET
			name = ?, description = ?, agent_ref = ?, model_override = ?, enabled = ?,
			start_at = ?, end_at = ?, schedule_type = ?, cron_expr = ?, interval_seconds = ?, timezone = ?, timeout_seconds = ?,
			task_prompt_uri = ?, task_prompt = ?, next_run_at = ?, last_run_at = ?, last_status = ?, last_error = ?,
			lease_owner = ?, lease_until = ?, created_at = ?, updated_at = ?
		WHERE id = ?`,
		rec.Name, rec.Description, rec.AgentRef, rec.ModelOverride, rec.Enabled,
		rec.StartAt, rec.EndAt, rec.ScheduleType, rec.CronExpr, rec.IntervalSeconds, rec.Timezone, rec.TimeoutSeconds,
		rec.TaskPromptUri, rec.TaskPrompt, rec.NextRunAt, rec.LastRunAt, rec.LastStatus, rec.LastError,
		rec.LeaseOwner, rec.LeaseUntil, rec.CreatedAt, rec.UpdatedAt, rec.Id,
	)
	return err
}
func (s *sqliteScheduleSQLX) Delete(tableName string, data interface{}) error { return nil }
func (s *sqliteScheduleSQLX) Execute(DML string, options ...sqlx.ExecutorOption) error {
	return nil
}
func (s *sqliteScheduleSQLX) Read(ctx context.Context, dest interface{}, SQL string, params ...interface{}) error {
	return nil
}
func (s *sqliteScheduleSQLX) Db(ctx context.Context) (*sql.DB, error) { return s.db, nil }
func (s *sqliteScheduleSQLX) Tx(ctx context.Context) (*sql.Tx, error) { return s.db.BeginTx(ctx, nil) }
func (s *sqliteScheduleSQLX) Validator() *validator.Service           { return validator.New(&fakeValidator{}) }

func seedScheduleSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	dbtest.LoadSQLiteSchema(t, db)
}
