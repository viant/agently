package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/memory"
)

func TestService_addUserMessageRawContent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cases := []struct {
		name      string
		raw       string
		expectRaw *string
	}{
		{name: "raw preserved", raw: "  original user input  ", expectRaw: ptr("  original user input  ")},
		{name: "whitespace raw ignored", raw: "   ", expectRaw: nil},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			turn := &memory.TurnMeta{ConversationID: "c1", TurnID: "t1", ParentMessageID: "p1"}
			recorder := &recordingConvClient{}
			svc := &Service{conversation: recorder}
			err := svc.addUserMessage(ctx, turn, "user-1", "expanded text", tc.raw)
			require.NoError(t, err)
			require.NotNil(t, recorder.lastMessage)
			assert.Equal(t, "expanded text", recorder.lastMessage.Content)
			if tc.expectRaw == nil {
				assert.Nil(t, recorder.lastMessage.RawContent)
			} else {
				if assert.NotNil(t, recorder.lastMessage.RawContent) {
					assert.Equal(t, *tc.expectRaw, *recorder.lastMessage.RawContent)
				}
			}
		})
	}
}

func ptr(v string) *string { return &v }

type recordingConvClient struct {
	lastMessage *apiconv.MutableMessage
}

func (r *recordingConvClient) GetConversation(ctx context.Context, id string, options ...apiconv.Option) (*apiconv.Conversation, error) {
	return nil, nil
}

func (r *recordingConvClient) GetConversations(ctx context.Context, input *apiconv.Input) ([]*apiconv.Conversation, error) {
	return nil, nil
}

func (r *recordingConvClient) PatchConversations(ctx context.Context, conversations *apiconv.MutableConversation) error {
	return nil
}

func (r *recordingConvClient) GetPayload(ctx context.Context, id string) (*apiconv.Payload, error) {
	return nil, nil
}

func (r *recordingConvClient) PatchPayload(ctx context.Context, payload *apiconv.MutablePayload) error {
	return nil
}

func (r *recordingConvClient) PatchMessage(ctx context.Context, message *apiconv.MutableMessage) error {
	copy := *message
	r.lastMessage = &copy
	return nil
}

func (r *recordingConvClient) GetMessage(ctx context.Context, id string, options ...apiconv.Option) (*apiconv.Message, error) {
	return nil, nil
}

func (r *recordingConvClient) GetMessageByElicitation(ctx context.Context, conversationID, elicitationID string) (*apiconv.Message, error) {
	return nil, nil
}

func (r *recordingConvClient) PatchModelCall(ctx context.Context, modelCall *apiconv.MutableModelCall) error {
	return nil
}

func (r *recordingConvClient) PatchToolCall(ctx context.Context, toolCall *apiconv.MutableToolCall) error {
	return nil
}

func (r *recordingConvClient) PatchTurn(ctx context.Context, turn *apiconv.MutableTurn) error {
	return nil
}

func (r *recordingConvClient) DeleteConversation(ctx context.Context, id string) error {
	return nil
}

func (r *recordingConvClient) DeleteMessage(ctx context.Context, conversationID, messageID string) error {
	return nil
}
