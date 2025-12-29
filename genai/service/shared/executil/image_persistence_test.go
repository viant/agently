package executil

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/memory"
	convmem "github.com/viant/agently/internal/service/conversation/memory"
	agconv "github.com/viant/agently/pkg/agently/conversation"
)

func TestPersistToolImageAttachmentIfNeeded_DataDriven(t *testing.T) {
	type testCase struct {
		name           string
		toolName       string
		result         any
		expectAttached bool
	}
	pngBytes := []byte("not-a-real-png-but-ok-for-persistence-test")

	cases := []testCase{
		{
			name:     "system-image-readImage",
			toolName: "system_image-readImage",
			result: map[string]any{
				"uri":        "file:///tmp/example.png",
				"mimeType":   "image/png",
				"name":       "example.png",
				"dataBase64": base64.StdEncoding.EncodeToString(pngBytes),
			},
			expectAttached: true,
		},
		{
			name:     "resources-readImage",
			toolName: "resources-readImage",
			result: map[string]any{
				"uri":        "file:///tmp/example.png",
				"mimeType":   "image/png",
				"name":       "example.png",
				"dataBase64": base64.StdEncoding.EncodeToString(pngBytes),
			},
			expectAttached: true,
		},
		{
			name:           "other-tool-noop",
			toolName:       "system_os-getEnv",
			result:         map[string]any{"names": []string{"HOME"}},
			expectAttached: false,
		},
	}

	for _, tc := range cases {
		ctx := context.Background()
		conv := convmem.New()
		convID := uuid.NewString()
		turnID := uuid.NewString()

		c := apiconv.NewConversation()
		c.SetId(convID)
		require.NoError(t, conv.PatchConversations(ctx, c), tc.name)

		turn := memory.TurnMeta{ConversationID: convID, TurnID: turnID}
		parentMsgID := uuid.NewString()
		_, err := apiconv.AddMessage(ctx, conv, &turn,
			apiconv.WithId(parentMsgID),
			apiconv.WithRole("tool"),
			apiconv.WithType("tool_op"),
			apiconv.WithContent("call-1"),
		)
		require.NoError(t, err, tc.name)

		raw, err := json.Marshal(tc.result)
		require.NoError(t, err, tc.name)

		require.NoError(t, persistToolImageAttachmentIfNeeded(ctx, conv, turn, parentMsgID, tc.toolName, string(raw)), tc.name)

		gotConv, err := conv.GetConversation(ctx, convID)
		require.NoError(t, err, tc.name)
		require.NotNil(t, gotConv, tc.name)

		var attachmentMessage *agconv.MessageView
		for _, trn := range gotConv.GetTranscript() {
			for _, msg := range trn.Message {
				if msg == nil || msg.ParentMessageId == nil {
					continue
				}
				if *msg.ParentMessageId == parentMsgID && msg.Role == "tool" && msg.Type == "control" {
					attachmentMessage = msg
					break
				}
			}
		}

		if !tc.expectAttached {
			assert.EqualValues(t, (*agconv.MessageView)(nil), attachmentMessage, tc.name)
			continue
		}
		require.NotNil(t, attachmentMessage, tc.name)
		require.NotNil(t, attachmentMessage.AttachmentPayloadId, tc.name)

		p, err := conv.GetPayload(ctx, *attachmentMessage.AttachmentPayloadId)
		require.NoError(t, err, tc.name)
		require.NotNil(t, p, tc.name)
		require.NotNil(t, p.InlineBody, tc.name)
		assert.EqualValues(t, pngBytes, *p.InlineBody, tc.name)
	}
}
