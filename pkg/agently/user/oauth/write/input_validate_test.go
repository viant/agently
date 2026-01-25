package write

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
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

func TestInput_Validate(t *testing.T) {
	tests := []struct {
		name             string
		input            *Input
		validator        *fakeValidator
		expectViolations int
	}{
		{
			name:      "no token no violations",
			input:     &Input{Token: nil},
			validator: &fakeValidator{},
		},
		{
			name:             "violation appended",
			input:            &Input{Token: &Token{UserID: "u1", Provider: "oauth", EncToken: "enc"}},
			validator:        &fakeValidator{violate: true},
			expectViolations: 1,
		},
		{
			name:             "update allows marker",
			input:            &Input{Token: &Token{UserID: "u1", Provider: "oauth", EncToken: "enc"}, CurToken: &Token{UserID: "u1", Provider: "oauth"}},
			validator:        &fakeValidator{violate: true},
			expectViolations: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess := newValidateSession(tt.input, tt.validator)
			out := &Output{}
			err := tt.input.Validate(context.Background(), sess, out)
			require.NoError(t, err)
			require.Len(t, out.Violations, tt.expectViolations)
		})
	}
}

type validateSession struct {
	input     *Input
	validator *validator.Service
}

func newValidateSession(input *Input, v *fakeValidator) *validateSession {
	return &validateSession{input: input, validator: validator.New(v)}
}

func (s *validateSession) Validator() *validator.Service { return s.validator }
func (s *validateSession) Differ() *differ.Service       { return nil }
func (s *validateSession) MessageBus() *mbus.Service     { return nil }
func (s *validateSession) Db(opts ...sqlx.Option) (*sqlx.Service, error) {
	return sqlx.New(&stubSQLX{}), nil
}
func (s *validateSession) Stater() *state.Service { return state.New(&stubInjector{input: s.input}) }
func (s *validateSession) FlushTemplate(context.Context) error {
	return nil
}
func (s *validateSession) Session(ctx context.Context, route *hhttp.Route, opts ...state.Option) (handler.Session, error) {
	return s, nil
}
func (s *validateSession) Http() hhttp.Http { return nil }
func (s *validateSession) Auth() auth.Auth  { return nil }
func (s *validateSession) Logger() logger.Logger {
	return nil
}

type stubSQLX struct{}

func (s *stubSQLX) Allocate(ctx context.Context, tableName string, dest interface{}, selector string) error {
	return nil
}
func (s *stubSQLX) Load(ctx context.Context, tableName string, data interface{}) error {
	return nil
}
func (s *stubSQLX) Flush(ctx context.Context, tableName string) error { return nil }
func (s *stubSQLX) Insert(tableName string, data interface{}) error   { return nil }
func (s *stubSQLX) Update(tableName string, data interface{}) error   { return nil }
func (s *stubSQLX) Delete(tableName string, data interface{}) error   { return nil }
func (s *stubSQLX) Execute(DML string, options ...sqlx.ExecutorOption) error {
	return nil
}
func (s *stubSQLX) Read(ctx context.Context, dest interface{}, SQL string, params ...interface{}) error {
	return nil
}
func (s *stubSQLX) Db(ctx context.Context) (*sql.DB, error) { return nil, nil }
func (s *stubSQLX) Tx(ctx context.Context) (*sql.Tx, error) { return nil, nil }
func (s *stubSQLX) Validator() *validator.Service           { return validator.New(&fakeValidator{}) }
