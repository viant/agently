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
	"github.com/viant/xdatly/handler/response"
	"github.com/viant/xdatly/handler/sqlx"
	"github.com/viant/xdatly/handler/state"
	"github.com/viant/xdatly/handler/validator"
	_ "modernc.org/sqlite"
)

func TestHandler_Exec_UserWrite(t *testing.T) {
	type expected struct {
		status       string
		inserted     bool
		updated      bool
		disabled     int
		createdSet   bool
		updatedSet   bool
		expectErr    bool
		expectErr400 bool
	}

	type testCase struct {
		name      string
		input     *Input
		seed      func(t *testing.T, db *sql.DB)
		validator *fakeValidator
		expect    expected
	}

	now := time.Now().UTC().Add(-2 * time.Hour)

	cases := []testCase{
		{
			name: "insert sets created_at and disabled default",
			input: &Input{Users: []*User{{
				Id:       "u1",
				Username: "alice",
				Provider: "local",
				Timezone: "UTC",
			}}},
			expect: expected{
				status:     "ok",
				inserted:   true,
				disabled:   0,
				createdSet: true,
				updatedSet: false,
			},
		},
		{
			name: "update stamps updated_at",
			input: &Input{
				Users: []*User{{
					Id:          "u2",
					Username:    "bob",
					Provider:    "oauth",
					Timezone:    "UTC",
					CreatedAt:   &now,
					DisplayName: ptr("Bob"),
					Disabled:    intPtr(0),
				}},
				CurUser: []*User{{Id: "u2"}},
			},
			seed: func(t *testing.T, db *sql.DB) {
				t.Helper()
				_, err := db.Exec(`INSERT INTO users (id, username, provider, timezone, disabled, created_at) VALUES (?, ?, ?, ?, 0, ?)`,
					"u2", "bob-old", "oauth", "UTC", now)
				require.NoError(t, err)
			},
			expect: expected{
				status:     "ok",
				updated:    true,
				createdSet: true,
				updatedSet: true,
			},
		},
		{
			name: "validation violation prevents write",
			input: &Input{Users: []*User{{
				Id:       "u3",
				Username: "eve",
				Provider: "local",
				Timezone: "UTC",
			}}},
			validator: &fakeValidator{violate: true},
			expect: expected{
				status:       "error",
				expectErr:    true,
				expectErr400: true,
				inserted:     false,
				updated:      false,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, _, cleanup := dbtest.CreateTempSQLiteDB(t, "agently-user-write")
			t.Cleanup(cleanup)
			seedUserSchema(t, db)

			if tc.seed != nil {
				tc.seed(t, db)
			}

			sqlxSvc := sqlx.New(&sqliteUserSQLX{db: db})
			v := tc.validator
			if v == nil {
				v = &fakeValidator{}
			}
			sess := newSQLiteSession(tc.input, sqlxSvc, validator.New(v))

			outAny, err := (&Handler{}).Exec(context.Background(), sess)
			if tc.expect.expectErr {
				require.Error(t, err)
				if tc.expect.expectErr400 {
					var rErr *response.Error
					require.True(t, errors.As(err, &rErr))
					require.Equal(t, 400, rErr.StatusCode())
				}
			} else {
				require.NoError(t, err)
			}

			out, ok := outAny.(*Output)
			require.True(t, ok)
			require.Equal(t, tc.expect.status, out.Status.Status)

			count := countUsers(t, db)
			if tc.expect.inserted || tc.expect.updated {
				require.Equal(t, 1, count)
				row := fetchUserRow(t, db, tc.input.Users[0].Id)
				require.Equal(t, tc.input.Users[0].Username, row.username)
				if tc.expect.createdSet {
					require.True(t, row.createdAt.Valid)
				}
				if tc.expect.updatedSet {
					require.True(t, row.updatedAt.Valid)
				} else {
					require.False(t, row.updatedAt.Valid)
				}
				if tc.expect.inserted {
					require.Equal(t, tc.expect.disabled, row.disabled)
				}
			} else {
				require.Equal(t, 0, count)
			}
		})
	}
}

type sqliteUserSQLX struct {
	db *sql.DB
}

func (s *sqliteUserSQLX) Allocate(ctx context.Context, tableName string, dest interface{}, selector string) error {
	return nil
}

func (s *sqliteUserSQLX) Load(ctx context.Context, tableName string, data interface{}) error {
	return nil
}

func (s *sqliteUserSQLX) Flush(ctx context.Context, tableName string) error {
	return nil
}

func (s *sqliteUserSQLX) Insert(tableName string, data interface{}) error {
	user, ok := data.(*User)
	if !ok {
		return errors.New("invalid insert data")
	}
	_, err := s.db.Exec(`
		INSERT INTO users
			(id, username, display_name, email, provider, subject, hash_ip, timezone,
			 default_agent_ref, default_model_ref, default_embedder_ref, settings,
			 disabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		user.Id, user.Username, user.DisplayName, user.Email, user.Provider, user.Subject, user.HashIP, user.Timezone,
		user.DefaultAgentRef, user.DefaultModelRef, user.DefaultEmbedderRef, user.Settings, user.Disabled,
		user.CreatedAt, user.UpdatedAt,
	)
	return err
}

func (s *sqliteUserSQLX) Update(tableName string, data interface{}) error {
	user, ok := data.(*User)
	if !ok {
		return errors.New("invalid update data")
	}
	_, err := s.db.Exec(`
		UPDATE users SET
			username = ?, display_name = ?, email = ?, provider = ?, subject = ?, hash_ip = ?, timezone = ?,
			default_agent_ref = ?, default_model_ref = ?, default_embedder_ref = ?, settings = ?, disabled = ?,
			created_at = ?, updated_at = ?
		WHERE id = ?`,
		user.Username, user.DisplayName, user.Email, user.Provider, user.Subject, user.HashIP, user.Timezone,
		user.DefaultAgentRef, user.DefaultModelRef, user.DefaultEmbedderRef, user.Settings, user.Disabled,
		user.CreatedAt, user.UpdatedAt, user.Id,
	)
	return err
}

func (s *sqliteUserSQLX) Delete(tableName string, data interface{}) error { return nil }
func (s *sqliteUserSQLX) Execute(DML string, options ...sqlx.ExecutorOption) error {
	return nil
}
func (s *sqliteUserSQLX) Read(ctx context.Context, dest interface{}, SQL string, params ...interface{}) error {
	return nil
}
func (s *sqliteUserSQLX) Db(ctx context.Context) (*sql.DB, error) { return s.db, nil }
func (s *sqliteUserSQLX) Tx(ctx context.Context) (*sql.Tx, error) { return s.db.BeginTx(ctx, nil) }
func (s *sqliteUserSQLX) Validator() *validator.Service           { return validator.New(&fakeValidator{}) }

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
		options.WithValidation.Append(options.Location, "username", "dup", "unique", "")
	}
	return options.WithValidation, v.err
}

type userRow struct {
	username  string
	disabled  int
	createdAt sql.NullString
	updatedAt sql.NullString
}

func fetchUserRow(t *testing.T, db *sql.DB, id string) userRow {
	t.Helper()
	row := userRow{}
	err := db.QueryRow(`SELECT username, disabled, created_at, updated_at FROM users WHERE id = ?`, id).Scan(
		&row.username, &row.disabled, &row.createdAt, &row.updatedAt,
	)
	require.NoError(t, err)
	return row
}

func countUsers(t *testing.T, db *sql.DB) int {
	t.Helper()
	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(1) FROM users`).Scan(&count))
	return count
}

func ptr(s string) *string { return &s }
func intPtr(v int) *int    { return &v }

func seedUserSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	dbtest.LoadSQLiteSchema(t, db)
}
