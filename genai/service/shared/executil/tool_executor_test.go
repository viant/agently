package executil

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/memory"
	"io"
)

type fakeConv struct {
	lastToolCall *apiconv.MutableToolCall
}

func (f *fakeConv) GetConversation(ctx context.Context, id string, options ...apiconv.Option) (*apiconv.Conversation, error) {
	return nil, nil
}
func (f *fakeConv) GetConversations(ctx context.Context) ([]*apiconv.Conversation, error) {
	return nil, nil
}
func (f *fakeConv) PatchConversations(ctx context.Context, conversations *apiconv.MutableConversation) error {
	return nil
}
func (f *fakeConv) GetPayload(ctx context.Context, id string) (*apiconv.Payload, error) {
	return nil, nil
}
func (f *fakeConv) PatchPayload(ctx context.Context, payload *apiconv.MutablePayload) error {
	return nil
}
func (f *fakeConv) PatchMessage(ctx context.Context, message *apiconv.MutableMessage) error {
	return nil
}
func (f *fakeConv) PatchModelCall(ctx context.Context, modelCall *apiconv.MutableModelCall) error {
	return nil
}
func (f *fakeConv) PatchToolCall(ctx context.Context, toolCall *apiconv.MutableToolCall) error {
	// capture every patch; completion should be the last
	cpy := *toolCall
	f.lastToolCall = &cpy
	return nil
}
func (f *fakeConv) PatchTurn(ctx context.Context, turn *apiconv.MutableTurn) error { return nil }

type fakeReg struct{ err error }

func (r *fakeReg) Definitions() []llm.ToolDefinition                     { return nil }
func (r *fakeReg) MatchDefinition(pattern string) []*llm.ToolDefinition  { return nil }
func (r *fakeReg) GetDefinition(name string) (*llm.ToolDefinition, bool) { return nil, false }
func (r *fakeReg) MustHaveTools(patterns []string) ([]llm.Tool, error)   { return nil, nil }
func (r *fakeReg) Execute(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	if r.err != nil {
		return "", r.err
	}
	return "ok", nil
}
func (r *fakeReg) SetDebugLogger(w io.Writer) {}

func TestExecuteToolStep_ErrorRecording(t *testing.T) {
	ctx := context.Background()
	// attach TurnMeta
	ctx = memory.WithTurnMeta(ctx, memory.TurnMeta{TurnID: "tid", ConversationID: "cid", ParentMessageID: "pid"})

	client := &fakeConv{}
	step := StepInfo{ID: "s1", Name: "demo", Args: map[string]interface{}{"a": 1}}

	cases := []struct {
		name       string
		regErr     error
		wantStatus string
		wantErrMsg bool
	}{
		{name: "success", regErr: nil, wantStatus: "completed", wantErrMsg: false},
		{name: "failure", regErr: errors.New("boom"), wantStatus: "failed", wantErrMsg: true},
	}
	for _, tc := range cases {
		reg := &fakeReg{err: tc.regErr}
		_, span, err := ExecuteToolStep(ctx, reg, step, client)
		if tc.regErr == nil {
			assert.NoError(t, err, tc.name)
		} else {
			assert.Error(t, err, tc.name)
		}
		// ensure span has times
		assert.False(t, span.StartedAt.IsZero(), tc.name)
		assert.False(t, span.EndedAt.IsZero(), tc.name)

		// check last tool call patch
		if assert.NotNil(t, client.lastToolCall, tc.name) {
			assert.EqualValues(t, tc.wantStatus, client.lastToolCall.Status, tc.name)
			if tc.wantErrMsg {
				if assert.NotNil(t, client.lastToolCall.ErrorMessage, tc.name) {
					assert.EqualValues(t, "boom", *client.lastToolCall.ErrorMessage, tc.name)
				}
			} else {
				assert.Nil(t, client.lastToolCall.ErrorMessage, tc.name)
			}
		}
	}
}
