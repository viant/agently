package agent

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiconv "github.com/viant/agently/client/conversation"
	agconv "github.com/viant/agently/pkg/agently/conversation"
)

type stubConversationClient struct {
	payloads map[string]*apiconv.Payload
}

func (s *stubConversationClient) GetPayload(ctx context.Context, id string) (*apiconv.Payload, error) {
	if s.payloads == nil {
		return nil, nil
	}
	return s.payloads[id], nil
}

func (s *stubConversationClient) GetConversation(ctx context.Context, id string, options ...apiconv.Option) (*apiconv.Conversation, error) {
	return nil, fmt.Errorf("not implemented")
}
func (s *stubConversationClient) GetConversations(ctx context.Context, input *apiconv.Input) ([]*apiconv.Conversation, error) {
	return nil, fmt.Errorf("not implemented")
}
func (s *stubConversationClient) PatchConversations(ctx context.Context, conversations *apiconv.MutableConversation) error {
	return fmt.Errorf("not implemented")
}
func (s *stubConversationClient) PatchPayload(ctx context.Context, payload *apiconv.MutablePayload) error {
	return fmt.Errorf("not implemented")
}
func (s *stubConversationClient) PatchMessage(ctx context.Context, message *apiconv.MutableMessage) error {
	return fmt.Errorf("not implemented")
}
func (s *stubConversationClient) GetMessage(ctx context.Context, id string, options ...apiconv.Option) (*apiconv.Message, error) {
	return nil, fmt.Errorf("not implemented")
}
func (s *stubConversationClient) GetMessageByElicitation(ctx context.Context, conversationID, elicitationID string) (*apiconv.Message, error) {
	return nil, fmt.Errorf("not implemented")
}
func (s *stubConversationClient) PatchModelCall(ctx context.Context, modelCall *apiconv.MutableModelCall) error {
	return fmt.Errorf("not implemented")
}
func (s *stubConversationClient) PatchToolCall(ctx context.Context, toolCall *apiconv.MutableToolCall) error {
	return fmt.Errorf("not implemented")
}
func (s *stubConversationClient) PatchTurn(ctx context.Context, turn *apiconv.MutableTurn) error {
	return fmt.Errorf("not implemented")
}
func (s *stubConversationClient) DeleteConversation(ctx context.Context, id string) error {
	return fmt.Errorf("not implemented")
}
func (s *stubConversationClient) DeleteMessage(ctx context.Context, conversationID, messageID string) error {
	return fmt.Errorf("not implemented")
}

func TestHistoryAttachmentCarriers_DataDriven(t *testing.T) {
	now := time.Now().UTC()
	payloadID := "payload-1"
	payloadBytes := []byte{0x01, 0x02, 0x03}

	type testCase struct {
		name            string
		carrierRole     string
		carrierFirst    bool
		expectMsgCount  int
		expectAttName   string
		expectAttURI    string
		expectAttMIME   string
		expectAttBytes  []byte
		expectCarrierIn bool
	}

	testCases := []testCase{
		{
			name:           "merges user control attachment",
			carrierRole:    "user",
			carrierFirst:   false,
			expectMsgCount: 1,
			expectAttName:  "img.png",
			expectAttURI:   "file:///tmp/img.png",
			expectAttMIME:  "image/png",
			expectAttBytes: payloadBytes,
		},
		{
			name:           "merges tool control attachment",
			carrierRole:    "tool",
			carrierFirst:   false,
			expectMsgCount: 1,
			expectAttName:  "img.png",
			expectAttURI:   "file:///tmp/img.png",
			expectAttMIME:  "image/png",
			expectAttBytes: payloadBytes,
		},
		{
			name:           "defers merge when carrier precedes parent",
			carrierRole:    "tool",
			carrierFirst:   true,
			expectMsgCount: 1,
			expectAttName:  "img.png",
			expectAttURI:   "file:///tmp/img.png",
			expectAttMIME:  "image/png",
			expectAttBytes: payloadBytes,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{
				conversation: &stubConversationClient{
					payloads: map[string]*apiconv.Payload{
						payloadID: {
							Id:         payloadID,
							MimeType:   "image/png",
							InlineBody: &payloadBytes,
							URI:        strPtr("file:///tmp/img.png"),
						},
					},
				},
			}

			parent := &apiconv.Message{
				Id:        "msg-user",
				Role:      "user",
				Type:      "text",
				Content:   strPtr("Task: analyze image"),
				CreatedAt: now,
			}
			carrier := &apiconv.Message{
				Id:                  "msg-att",
				Role:                tc.carrierRole,
				Type:                "control",
				Content:             strPtr("img.png"),
				CreatedAt:           now.Add(time.Millisecond),
				ParentMessageId:     strPtr(parent.Id),
				AttachmentPayloadId: strPtr(payloadID),
			}

			turnMsgs := []*agconv.MessageView{(*agconv.MessageView)(parent), (*agconv.MessageView)(carrier)}
			if tc.carrierFirst {
				turnMsgs = []*agconv.MessageView{(*agconv.MessageView)(carrier), (*agconv.MessageView)(parent)}
			}
			turn := &apiconv.Turn{
				Id:      "turn-1",
				Message: turnMsgs,
			}

			hist, err := svc.buildHistory(context.Background(), apiconv.Transcript{turn})
			require.NoError(t, err)
			require.Len(t, hist.Past, 1)
			require.Len(t, hist.Past[0].Messages, tc.expectMsgCount)

			got := hist.Past[0].Messages[0]
			require.NotNil(t, got)
			require.Len(t, got.Attachment, 1)
			assert.EqualValues(t, tc.expectAttName, got.Attachment[0].Name)
			assert.EqualValues(t, tc.expectAttURI, got.Attachment[0].URI)
			assert.EqualValues(t, tc.expectAttMIME, got.Attachment[0].Mime)
			assert.EqualValues(t, tc.expectAttBytes, got.Attachment[0].Data)
		})
	}
}
