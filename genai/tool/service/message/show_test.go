package message

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/textclip"
)

// fakeConv implements a minimal conversation client returning static messages.
type fakeConv struct {
	msgs map[string]*apiconv.Message
}

func (f *fakeConv) GetConversation(context.Context, string, ...apiconv.Option) (*apiconv.Conversation, error) {
	return nil, nil
}
func (f *fakeConv) GetConversations(context.Context, *apiconv.Input) ([]*apiconv.Conversation, error) {
	return nil, nil
}
func (f *fakeConv) PatchConversations(context.Context, *apiconv.MutableConversation) error {
	return nil
}
func (f *fakeConv) GetPayload(context.Context, string) (*apiconv.Payload, error) { return nil, nil }
func (f *fakeConv) PatchPayload(context.Context, *apiconv.MutablePayload) error  { return nil }
func (f *fakeConv) PatchMessage(context.Context, *apiconv.MutableMessage) error  { return nil }
func (f *fakeConv) GetMessage(_ context.Context, id string, _ ...apiconv.Option) (*apiconv.Message, error) {
	if f.msgs == nil {
		return nil, nil
	}
	return f.msgs[id], nil
}
func (f *fakeConv) GetMessageByElicitation(context.Context, string, string) (*apiconv.Message, error) {
	return nil, nil
}
func (f *fakeConv) PatchModelCall(context.Context, *apiconv.MutableModelCall) error { return nil }
func (f *fakeConv) PatchToolCall(context.Context, *apiconv.MutableToolCall) error   { return nil }
func (f *fakeConv) PatchTurn(context.Context, *apiconv.MutableTurn) error           { return nil }
func (f *fakeConv) DeleteConversation(context.Context, string) error                { return nil }
func (f *fakeConv) DeleteMessage(context.Context, string, string) error             { return nil }

func TestShow_TransformAndRanges(t *testing.T) {
	mem := &fakeConv{msgs: map[string]*apiconv.Message{}}
	svc := New(mem)
	tests := []struct {
		name        string
		body        string
		input       ShowInput
		wantContent string
		wantOffset  int
	}{
		{
			name:        "no transform full body",
			body:        "hello\nworld\n",
			input:       ShowInput{},
			wantContent: "hello\nworld\n",
			wantOffset:  0,
		},
		{
			name:        "byte range",
			body:        "abcdef",
			input:       ShowInput{ByteRange: &textclip.IntRange{From: intPtr(2), To: intPtr(5)}},
			wantContent: "cde",
			wantOffset:  2,
		},
		{
			name:        "sed replace",
			body:        "foo bar",
			input:       ShowInput{Sed: []string{"s/foo/baz/g"}},
			wantContent: "baz bar\n",
			wantOffset:  0,
		},
		{
			name:        "transform csv dot-path",
			body:        `{"data":[{"a":1,"b":"x"},{"a":2,"b":"y"}]}`,
			input:       ShowInput{Transform: &TransformSpec{Selector: "data", Format: "csv", Fields: []string{"a", "b"}}},
			wantContent: "a,b\n1,x\n2,y\n",
			wantOffset:  0,
		},
		{
			name:        "transform ndjson object",
			body:        `{"a":1}`,
			input:       ShowInput{Transform: &TransformSpec{Format: "ndjson"}},
			wantContent: "{\"a\":1}\n",
			wantOffset:  0,
		},
		{
			name:        "transform csv with payload preface",
			body:        "status: ok\npayload: {\"data\":[{\"a\":1,\"b\":\"x\"},{\"a\":2,\"b\":\"y\"}]}",
			input:       ShowInput{Transform: &TransformSpec{Selector: "data", Format: "csv", Fields: []string{"a", "b"}}},
			wantContent: "a,b\n1,x\n2,y\n",
			wantOffset:  0,
		},
		{
			name:        "transform csv with fenced json",
			body:        "Here is the result:\n```json\n{\n  \"data\": [ { \"a\": 1, \"b\": \"x\" }, { \"a\": 2, \"b\": \"y\" } ]\n}\n```\nThanks",
			input:       ShowInput{Transform: &TransformSpec{Selector: "data", Format: "csv", Fields: []string{"a", "b"}}},
			wantContent: "a,b\n1,x\n2,y\n",
			wantOffset:  0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Prepare a synthetic message for this test case.
			msgID := "m1"
			body := tc.body
			mem.msgs[msgID] = &apiconv.Message{RawContent: &body}
			in := tc.input
			in.MessageID = msgID
			var out ShowOutput
			err := svc.show(context.Background(), &in, &out)
			assert.NoError(t, err)
			assert.EqualValues(t, tc.wantContent, out.Content)
			assert.EqualValues(t, tc.wantOffset, out.Offset)
		})
	}
}

func intPtr(i int) *int { return &i }
