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

type runSession struct {
	input     *Input
	sqlx      *sqlx.Service
	validator *validator.Service
}

func newRunSession(input *Input, db *sql.DB, v *fakeValidator) *runSession {
	return &runSession{
		input:     input,
		sqlx:      sqlx.New(&sqliteRunSQLX{db: db}),
		validator: validator.New(v),
	}
}

func (s *runSession) Validator() *validator.Service { return s.validator }
func (s *runSession) Differ() *differ.Service       { return nil }
func (s *runSession) MessageBus() *mbus.Service     { return nil }
func (s *runSession) Db(opts ...sqlx.Option) (*sqlx.Service, error) {
	return s.sqlx, nil
}
func (s *runSession) Stater() *state.Service { return state.New(&stubInjector{input: s.input}) }
func (s *runSession) FlushTemplate(context.Context) error {
	return nil
}
func (s *runSession) Session(ctx context.Context, route *hhttp.Route, opts ...state.Option) (handler.Session, error) {
	return s, nil
}
func (s *runSession) Http() hhttp.Http { return nil }
func (s *runSession) Auth() auth.Auth  { return nil }
func (s *runSession) Logger() logger.Logger {
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

type sqliteRunSQLX struct {
	db *sql.DB
}

func (s *sqliteRunSQLX) Allocate(ctx context.Context, tableName string, dest interface{}, selector string) error {
	return nil
}
func (s *sqliteRunSQLX) Load(ctx context.Context, tableName string, data interface{}) error {
	return nil
}
func (s *sqliteRunSQLX) Flush(ctx context.Context, tableName string) error { return nil }
func (s *sqliteRunSQLX) Insert(tableName string, data interface{}) error {
	rec, ok := data.(*Run)
	if !ok {
		return errors.New("invalid insert data")
	}
	_, err := s.db.Exec(`
		INSERT INTO schedule_run (
			id, schedule_id, created_at, updated_at, status, error_message,
			precondition_ran_at, precondition_passed, precondition_result,
			conversation_id, conversation_kind, scheduled_for, started_at, completed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.Id, rec.ScheduleId, rec.CreatedAt, rec.UpdatedAt, rec.Status, rec.ErrorMessage,
		rec.PreconditionRanAt, rec.PreconditionPassed, rec.PreconditionResult,
		rec.ConversationId, rec.ConversationKind, rec.ScheduledFor, rec.StartedAt, rec.CompletedAt,
	)
	return err
}
func (s *sqliteRunSQLX) Update(tableName string, data interface{}) error {
	rec, ok := data.(*Run)
	if !ok {
		return errors.New("invalid update data")
	}
	_, err := s.db.Exec(`
		UPDATE schedule_run SET
			schedule_id = ?, created_at = ?, updated_at = ?, status = ?, error_message = ?,
			precondition_ran_at = ?, precondition_passed = ?, precondition_result = ?,
			conversation_id = ?, conversation_kind = ?, scheduled_for = ?, started_at = ?, completed_at = ?
		WHERE id = ?`,
		rec.ScheduleId, rec.CreatedAt, rec.UpdatedAt, rec.Status, rec.ErrorMessage,
		rec.PreconditionRanAt, rec.PreconditionPassed, rec.PreconditionResult,
		rec.ConversationId, rec.ConversationKind, rec.ScheduledFor, rec.StartedAt, rec.CompletedAt,
		rec.Id,
	)
	return err
}
func (s *sqliteRunSQLX) Delete(tableName string, data interface{}) error { return nil }
func (s *sqliteRunSQLX) Execute(DML string, options ...sqlx.ExecutorOption) error {
	return nil
}
func (s *sqliteRunSQLX) Read(ctx context.Context, dest interface{}, SQL string, params ...interface{}) error {
	return nil
}
func (s *sqliteRunSQLX) Db(ctx context.Context) (*sql.DB, error) { return s.db, nil }
func (s *sqliteRunSQLX) Tx(ctx context.Context) (*sql.Tx, error) { return s.db.BeginTx(ctx, nil) }
func (s *sqliteRunSQLX) Validator() *validator.Service           { return validator.New(&fakeValidator{}) }

func seedRunSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	dbtest.LoadSQLiteSchema(t, db)
}
