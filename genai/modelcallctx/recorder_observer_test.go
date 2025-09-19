package modelcallctx

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	apiconv "github.com/viant/agently/client/conversation"
)

// fakeConvClient is a minimal fake implementing apiconv.Client for testing.
type fakeConvClient struct {
	payloads map[string]*apiconv.MutablePayload
}

func (f *fakeConvClient) ensure() {
	if f.payloads == nil {
		f.payloads = map[string]*apiconv.MutablePayload{}
	}
}

func (f *fakeConvClient) GetConversation(ctx context.Context, id string, options ...apiconv.Option) (*apiconv.Conversation, error) {
	return nil, nil
}
func (f *fakeConvClient) GetConversations(ctx context.Context) ([]*apiconv.Conversation, error) {
	return nil, nil
}
func (f *fakeConvClient) PatchConversations(ctx context.Context, conversations *apiconv.MutableConversation) error {
	return nil
}
func (f *fakeConvClient) GetPayload(ctx context.Context, id string) (*apiconv.Payload, error) {
	return nil, nil
}
func (f *fakeConvClient) PatchPayload(ctx context.Context, payload *apiconv.MutablePayload) error {
	f.ensure()
	// store by id
	f.payloads[payload.Id] = payload
	return nil
}
func (f *fakeConvClient) PatchMessage(ctx context.Context, message *apiconv.MutableMessage) error {
	return nil
}
func (f *fakeConvClient) PatchModelCall(ctx context.Context, modelCall *apiconv.MutableModelCall) error {
	return nil
}
func (f *fakeConvClient) PatchToolCall(ctx context.Context, toolCall *apiconv.MutableToolCall) error {
	return nil
}
func (f *fakeConvClient) PatchTurn(ctx context.Context, turn *apiconv.MutableTurn) error { return nil }

func TestUpsertInlinePayload(t *testing.T) {
	ctx := context.Background()
	client := &fakeConvClient{}
	rec := &recorderObserver{client: client}

	type testCase struct {
		name string
		id   string
		kind string
		mime string
		body []byte
	}
	cases := []testCase{
		{name: "new id", id: "", kind: "model_request", mime: "application/json", body: []byte(`{"a":1}`)},
		{name: "existing id", id: "fixed", kind: "model_stream", mime: "text/plain", body: []byte("hello")},
	}
	for _, tc := range cases {
		gotID, err := rec.upsertInlinePayload(ctx, tc.id, tc.kind, tc.mime, tc.body)
		if !assert.NoError(t, err, tc.name) {
			continue
		}
		// id is preserved if provided, otherwise generated
		if tc.id != "" {
			assert.EqualValues(t, tc.id, gotID, tc.name)
		} else {
			assert.NotEmpty(t, gotID, tc.name)
		}
		// payload stored with expected fields
		stored := client.payloads[gotID]
		if !assert.NotNil(t, stored, tc.name) {
			continue
		}
		assert.EqualValues(t, tc.kind, stored.Kind, tc.name)
		assert.EqualValues(t, tc.mime, stored.MimeType, tc.name)
		assert.EqualValues(t, len(tc.body), stored.SizeBytes, tc.name)
		if assert.NotNil(t, stored.InlineBody, tc.name) {
			assert.EqualValues(t, tc.body, *stored.InlineBody, tc.name)
		}
	}
}
