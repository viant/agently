package write

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
	_ "modernc.org/sqlite"
)

func TestHandler_Exec_SessionWrite(t *testing.T) {
	type expected struct {
		inserted   bool
		updated    bool
		createdSet bool
		updatedSet bool
		expectRows int
	}

	type testCase struct {
		name   string
		input  *Input
		seed   func(t *testing.T, db *sql.DB)
		expect expected
	}

	now := time.Now().UTC().Add(-2 * time.Hour)
	expires := time.Now().UTC().Add(2 * time.Hour)

	cases := []testCase{
		{
			name: "insert stamps created_at",
			input: &Input{Session: &Session{
				Id:        "s1",
				UserID:    "u1",
				Provider:  "local",
				ExpiresAt: expires,
			}},
			expect: expected{inserted: true, createdSet: true, expectRows: 1},
		},
		{
			name: "update stamps updated_at",
			input: &Input{
				Session: &Session{
					Id:        "s2",
					UserID:    "u2",
					Provider:  "oauth",
					CreatedAt: now,
					ExpiresAt: expires,
				},
				CurSession: &Session{Id: "s2"},
			},
			seed: func(t *testing.T, db *sql.DB) {
				t.Helper()
				_, err := db.Exec(
					`INSERT INTO session (id, user_id, provider, created_at, expires_at) VALUES (?, ?, ?, ?, ?)`,
					"s2", "u2", "oauth", now, expires,
				)
				require.NoError(t, err)
			},
			expect: expected{updated: true, createdSet: true, updatedSet: true, expectRows: 1},
		},
		{
			name:   "no session no writes",
			input:  &Input{Session: nil},
			expect: expected{expectRows: 0},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, _, cleanup := dbtest.CreateTempSQLiteDB(t, "agently-session-write")
			t.Cleanup(cleanup)
			seedSessionSchema(t, db)

			if tc.seed != nil {
				tc.seed(t, db)
			}

			sqlxSvc := sqlx.New(&sqliteSessionSQLX{db: db})
			sess := newSQLiteSession(tc.input, sqlxSvc, validator.New(&fakeValidator{}))

			outAny, err := (&Handler{}).Exec(context.Background(), sess)
			require.NoError(t, err)

			out, ok := outAny.(*Output)
			require.True(t, ok)
			require.Equal(t, "ok", out.Status.Status)

			count := countSessions(t, db)
			require.Equal(t, tc.expect.expectRows, count)
			if tc.expect.inserted || tc.expect.updated {
				row := fetchSessionRow(t, db, tc.input.Session.Id)
				if tc.expect.createdSet {
					require.True(t, row.createdAt.Valid)
				}
				if tc.expect.updatedSet {
					require.True(t, row.updatedAt.Valid)
				} else {
					require.False(t, row.updatedAt.Valid)
				}
			}
		})
	}
}

type sqliteSessionSQLX struct {
	db *sql.DB
}

func (s *sqliteSessionSQLX) Allocate(ctx context.Context, tableName string, dest interface{}, selector string) error {
	return nil
}
func (s *sqliteSessionSQLX) Load(ctx context.Context, tableName string, data interface{}) error {
	return nil
}
func (s *sqliteSessionSQLX) Flush(ctx context.Context, tableName string) error { return nil }
func (s *sqliteSessionSQLX) Insert(tableName string, data interface{}) error {
	rec, ok := data.(*Session)
	if !ok {
		return errors.New("invalid insert data")
	}
	_, err := s.db.Exec(`
		INSERT INTO session (id, user_id, provider, created_at, updated_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		rec.Id, rec.UserID, rec.Provider, rec.CreatedAt, rec.UpdatedAt, rec.ExpiresAt,
	)
	return err
}
func (s *sqliteSessionSQLX) Update(tableName string, data interface{}) error {
	rec, ok := data.(*Session)
	if !ok {
		return errors.New("invalid update data")
	}
	_, err := s.db.Exec(`
		UPDATE session SET
			user_id = ?, provider = ?, created_at = ?, updated_at = ?, expires_at = ?
		WHERE id = ?`,
		rec.UserID, rec.Provider, rec.CreatedAt, rec.UpdatedAt, rec.ExpiresAt, rec.Id,
	)
	return err
}
func (s *sqliteSessionSQLX) Delete(tableName string, data interface{}) error { return nil }
func (s *sqliteSessionSQLX) Execute(DML string, options ...sqlx.ExecutorOption) error {
	return nil
}
func (s *sqliteSessionSQLX) Read(ctx context.Context, dest interface{}, SQL string, params ...interface{}) error {
	return nil
}
func (s *sqliteSessionSQLX) Db(ctx context.Context) (*sql.DB, error) { return s.db, nil }
func (s *sqliteSessionSQLX) Tx(ctx context.Context) (*sql.Tx, error) { return s.db.BeginTx(ctx, nil) }
func (s *sqliteSessionSQLX) Validator() *validator.Service           { return validator.New(&fakeValidator{}) }

type sqliteSession struct {
	input     *Input
	sqlx      *sqlx.Service
	validator *validator.Service
}

func newSQLiteSession(input *Input, sqlxSvc *sqlx.Service, v *validator.Service) *sqliteSession {
	return &sqliteSession{input: input, sqlx: sqlxSvc, validator: v}
}

func (s *sqliteSession) Validator() *validator.Service { return s.validator }
func (s *sqliteSession) Differ() *differ.Service       { return nil }
func (s *sqliteSession) MessageBus() *mbus.Service     { return nil }
func (s *sqliteSession) Db(opts ...sqlx.Option) (*sqlx.Service, error) {
	return s.sqlx, nil
}
func (s *sqliteSession) Stater() *state.Service { return state.New(&stubInjector{input: s.input}) }
func (s *sqliteSession) FlushTemplate(context.Context) error {
	return nil
}
func (s *sqliteSession) Session(ctx context.Context, route *hhttp.Route, opts ...state.Option) (handler.Session, error) {
	return s, nil
}
func (s *sqliteSession) Http() hhttp.Http { return nil }
func (s *sqliteSession) Auth() auth.Auth  { return nil }
func (s *sqliteSession) Logger() logger.Logger {
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

type fakeValidator struct{}

func (v *fakeValidator) Validate(ctx context.Context, any interface{}, opts ...validator.Option) (*validator.Validation, error) {
	options := &validator.Options{}
	options.Apply(opts)
	if options.WithValidation == nil {
		options.WithValidation = validator.NewValidation()
	}
	return options.WithValidation, nil
}

type sessionRow struct {
	createdAt sql.NullString
	updatedAt sql.NullString
}

func fetchSessionRow(t *testing.T, db *sql.DB, id string) sessionRow {
	t.Helper()
	row := sessionRow{}
	err := db.QueryRow(`SELECT created_at, updated_at FROM session WHERE id = ?`, id).Scan(
		&row.createdAt, &row.updatedAt,
	)
	require.NoError(t, err)
	return row
}

func countSessions(t *testing.T, db *sql.DB) int {
	t.Helper()
	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(1) FROM session`).Scan(&count))
	return count
}

func seedSessionSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	dbtest.LoadSQLiteSchema(t, db)
}
