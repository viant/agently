package conversation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	authctx "github.com/viant/agently/internal/auth"
)

func TestFilter_Compute_PrivacyAndScope(t *testing.T) {
	type testCase struct {
		name       string
		input      *ConversationInput
		userID     string
		wantExpr   string
		wantParams []interface{}
	}

	cases := []testCase{
		{
			name:       "history with identity includes public or own private",
			input:      &ConversationInput{Has: &ConversationInputHas{}},
			userID:     "user1",
			wantExpr:   "t.conversation_parent_id = '' AND t.schedule_id IS NULL AND (COALESCE(t.visibility, '') <> ? OR t.created_by_user_id = ?)",
			wantParams: []interface{}{"private", "user1"},
		},
		{
			name:       "single by id skips visibility filter",
			input:      &ConversationInput{Has: &ConversationInputHas{Id: true}},
			userID:     "user2",
			wantExpr:   "1=1",
			wantParams: nil,
		},
		{
			name:       "history without identity returns only non-private",
			input:      &ConversationInput{Has: &ConversationInputHas{}},
			userID:     "",
			wantExpr:   "t.conversation_parent_id = '' AND t.schedule_id IS NULL AND COALESCE(t.visibility, '') <> ?",
			wantParams: []interface{}{"private"},
		},
		{
		}
	}

	f := &Filter{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Build context with input and optional user info
			ctx := context.WithValue(context.Background(), inputKey, tc.input)
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
