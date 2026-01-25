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

func TestHandler_Exec_OAuthTokenWrite(t *testing.T) {
	type expected struct {
		status       string
		expectErr    bool
		expectErr400 bool
		expectRows   int
		createdSet   bool
		updatedSet   bool
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
			name: "insert sets created_at",
			input: &Input{Token: &Token{
				UserID:   "u1",
				Provider: "oauth",
				EncToken: "enc1",
			}},
			expect: expected{status: "ok", expectRows: 1, createdSet: true},
		},
		{
			name: "update sets updated_at",
			input: &Input{
				Token:    &Token{UserID: "u2", Provider: "oauth", EncToken: "enc2", CreatedAt: now},
				CurToken: &Token{UserID: "u2", Provider: "oauth"},
			},
			seed: func(t *testing.T, db *sql.DB) {
				t.Helper()
				_, err := db.Exec(
					`INSERT INTO user_oauth_token (user_id, provider, enc_token, created_at) VALUES (?, ?, ?, ?)`,
					"u2", "oauth", "old", now,
				)
				require.NoError(t, err)
			},
			expect: expected{status: "ok", expectRows: 1, createdSet: true, updatedSet: true},
		},
		{
			name: "validation violation prevents write",
			input: &Input{Token: &Token{
				UserID:   "u3",
				Provider: "oauth",
				EncToken: "enc3",
			}},
			validator: &fakeValidator{violate: true},
			expect: expected{
				status:       "error",
				expectErr:    true,
				expectErr400: true,
				expectRows:   0,
			},
		},
		{
			name:   "no token no writes",
			input:  &Input{Token: nil},
			expect: expected{status: "ok", expectRows: 0},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, _, cleanup := dbtest.CreateTempSQLiteDB(t, "agently-oauth-write")
			t.Cleanup(cleanup)
			seedOAuthSchema(t, db)

			if tc.input != nil && tc.input.Token != nil {
				ensureUser(t, db, tc.input.Token.UserID)
			}

			if tc.seed != nil {
				tc.seed(t, db)
			}

			sqlxSvc := sqlx.New(&sqliteOAuthSQLX{db: db})
			v := tc.validator
			if v == nil {
				v = &fakeValidator{}
			}
			sess := newOAuthSession(tc.input, sqlxSvc, validator.New(v))

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

			count := countOAuthTokens(t, db)
			require.Equal(t, tc.expect.expectRows, count)
			if tc.expect.expectRows > 0 {
				row := fetchOAuthRow(t, db, tc.input.Token.UserID, tc.input.Token.Provider)
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

type sqliteOAuthSQLX struct {
	db *sql.DB
}

func (s *sqliteOAuthSQLX) Allocate(ctx context.Context, tableName string, dest interface{}, selector string) error {
	return nil
}
func (s *sqliteOAuthSQLX) Load(ctx context.Context, tableName string, data interface{}) error {
	return nil
}
func (s *sqliteOAuthSQLX) Flush(ctx context.Context, tableName string) error { return nil }
func (s *sqliteOAuthSQLX) Insert(tableName string, data interface{}) error {
	rec, ok := data.(*Token)
	if !ok {
		return errors.New("invalid insert data")
	}
	_, err := s.db.Exec(`
		INSERT INTO user_oauth_token (user_id, provider, enc_token, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`,
		rec.UserID, rec.Provider, rec.EncToken, rec.CreatedAt, rec.UpdatedAt,
	)
	return err
}
func (s *sqliteOAuthSQLX) Update(tableName string, data interface{}) error {
	rec, ok := data.(*Token)
	if !ok {
		return errors.New("invalid update data")
	}
	_, err := s.db.Exec(`
		UPDATE user_oauth_token SET
			enc_token = ?, created_at = ?, updated_at = ?
		WHERE user_id = ? AND provider = ?`,
		rec.EncToken, rec.CreatedAt, rec.UpdatedAt, rec.UserID, rec.Provider,
	)
	return err
}
func (s *sqliteOAuthSQLX) Delete(tableName string, data interface{}) error { return nil }
func (s *sqliteOAuthSQLX) Execute(DML string, options ...sqlx.ExecutorOption) error {
	return nil
}
func (s *sqliteOAuthSQLX) Read(ctx context.Context, dest interface{}, SQL string, params ...interface{}) error {
	return nil
}
func (s *sqliteOAuthSQLX) Db(ctx context.Context) (*sql.DB, error) { return s.db, nil }
func (s *sqliteOAuthSQLX) Tx(ctx context.Context) (*sql.Tx, error) { return s.db.BeginTx(ctx, nil) }
func (s *sqliteOAuthSQLX) Validator() *validator.Service           { return validator.New(&fakeValidator{}) }

type oauthSession struct {
	input     *Input
	sqlx      *sqlx.Service
	validator *validator.Service
}

func newOAuthSession(input *Input, sqlxSvc *sqlx.Service, v *validator.Service) *oauthSession {
	return &oauthSession{input: input, sqlx: sqlxSvc, validator: v}
}

func (s *oauthSession) Validator() *validator.Service { return s.validator }
func (s *oauthSession) Differ() *differ.Service       { return nil }
func (s *oauthSession) MessageBus() *mbus.Service     { return nil }
func (s *oauthSession) Db(opts ...sqlx.Option) (*sqlx.Service, error) {
	return s.sqlx, nil
}
func (s *oauthSession) Stater() *state.Service { return state.New(&stubInjector{input: s.input}) }
func (s *oauthSession) FlushTemplate(context.Context) error {
	return nil
}
func (s *oauthSession) Session(ctx context.Context, route *hhttp.Route, opts ...state.Option) (handler.Session, error) {
	return s, nil
}
func (s *oauthSession) Http() hhttp.Http { return nil }
func (s *oauthSession) Auth() auth.Auth  { return nil }
func (s *oauthSession) Logger() logger.Logger {
	return nil
}

type oauthRow struct {
	createdAt sql.NullString
	updatedAt sql.NullString
}

func fetchOAuthRow(t *testing.T, db *sql.DB, userID, provider string) oauthRow {
	t.Helper()
	row := oauthRow{}
	err := db.QueryRow(`SELECT created_at, updated_at FROM user_oauth_token WHERE user_id = ? AND provider = ?`, userID, provider).Scan(
		&row.createdAt, &row.updatedAt,
	)
	require.NoError(t, err)
	return row
}

func countOAuthTokens(t *testing.T, db *sql.DB) int {
	t.Helper()
	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(1) FROM user_oauth_token`).Scan(&count))
	return count
}

func seedOAuthSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	dbtest.LoadSQLiteSchema(t, db)
}

func ensureUser(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	_, err := db.Exec(
		`INSERT OR IGNORE INTO users (id, username, provider, timezone, disabled, created_at) VALUES (?, ?, 'local', 'UTC', 0, ?)`,
		id, id, time.Now().UTC(),
	)
	require.NoError(t, err)
}
