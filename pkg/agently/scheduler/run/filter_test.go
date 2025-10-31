package run

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	authctx "github.com/viant/agently/internal/auth"
)

func TestRunFilter_Compute_Privacy(t *testing.T) {
	type testCase struct {
		name       string
		userID     string
		wantExpr   string
		wantParams []interface{}
	}
	cases := []testCase{
		{
			name:       "anonymous sees only public-linked runs",
			userID:     "",
			wantExpr:   "t.conversation_id IS NOT NULL AND EXISTS (SELECT 1 FROM conversation c WHERE c.id = t.conversation_id AND COALESCE(c.visibility, '') <> ?)",
			wantParams: []interface{}{"private"},
		},
		{
			name:       "user sees public or own private runs",
			userID:     "alice",
			wantExpr:   "t.conversation_id IS NOT NULL AND EXISTS (SELECT 1 FROM conversation c WHERE c.id = t.conversation_id AND (COALESCE(c.visibility, '') <> ? OR c.created_by_user_id = ?))",
			wantParams: []interface{}{"private", "alice"},
		},
	}

	f := &Filter{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Build context with user info only (RunInput presence is not needed by Filter)
			ctx := context.Background()
			if tc.userID != "" {
				ctx = authctx.WithUserInfo(ctx, &authctx.UserInfo{Subject: tc.userID})
			}
			got, err := f.Compute(ctx, nil)
			assert.NoError(t, err)
			assert.NotNil(t, got)
			assert.EqualValues(t, tc.wantExpr, got.Expression)
			assert.EqualValues(t, tc.wantParams, got.Placeholders)
		})
	}
}
