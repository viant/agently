package write

import (
	"context"
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

func TestInput_Init(t *testing.T) {
	tests := []struct {
		name           string
		input          *Input
		expectCreated  bool
		expectUpdated  bool
		expectHasMarks bool
	}{
		{
			name:          "insert sets created_at",
			input:         &Input{Token: &Token{UserID: "u1", Provider: "oauth", EncToken: "enc"}},
			expectCreated: true,
		},
		{
			name: "update sets updated_at and markers",
			input: &Input{
				Token:    &Token{UserID: "u1", Provider: "oauth", EncToken: "enc"},
				CurToken: &Token{UserID: "u1", Provider: "oauth"},
			},
			expectUpdated:  true,
			expectHasMarks: true,
		},
		{
			name:  "nil token no-op",
			input: &Input{Token: nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess := newStubSession(tt.input)
			err := tt.input.Init(context.Background(), sess, &Output{})
			require.NoError(t, err)
			if tt.input.Token == nil {
				return
			}
			if tt.expectCreated {
				require.False(t, tt.input.Token.CreatedAt.IsZero())
			}
			if tt.expectUpdated {
				require.NotNil(t, tt.input.Token.UpdatedAt)
			}
			if tt.expectHasMarks {
				require.NotNil(t, tt.input.Token.Has)
				require.True(t, tt.input.Token.Has.UserID)
				require.True(t, tt.input.Token.Has.Provider)
				require.True(t, tt.input.Token.Has.EncToken)
			}
		})
	}
}

type stubSession struct {
	input *Input
}

func newStubSession(input *Input) *stubSession { return &stubSession{input: input} }

func (s *stubSession) Validator() *validator.Service { return validator.New(&fakeValidator{}) }
func (s *stubSession) Differ() *differ.Service       { return nil }
func (s *stubSession) MessageBus() *mbus.Service     { return nil }
func (s *stubSession) Db(opts ...sqlx.Option) (*sqlx.Service, error) {
	return nil, nil
}
func (s *stubSession) Stater() *state.Service { return state.New(&stubInjector{input: s.input}) }
func (s *stubSession) FlushTemplate(context.Context) error {
	return nil
}
func (s *stubSession) Session(ctx context.Context, route *hhttp.Route, opts ...state.Option) (handler.Session, error) {
	return s, nil
}
func (s *stubSession) Http() hhttp.Http { return nil }
func (s *stubSession) Auth() auth.Auth  { return nil }
func (s *stubSession) Logger() logger.Logger {
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
		options.WithValidation.Append(options.Location, "encToken", "dup", "unique", "")
	}
	return options.WithValidation, v.err
}
