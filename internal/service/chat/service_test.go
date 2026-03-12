package chat

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	apiconv "github.com/viant/agently/client/conversation"
	memconv "github.com/viant/agently/internal/service/conversation/memory"
)

type tolerantClient struct {
	*memconv.Client
	msgConversation  map[string]string
	msgTurn          map[string]string
	turnConversation map[string]string
}

func newTolerantClient() *tolerantClient {
	return &tolerantClient{
		Client:           memconv.New(),
		msgConversation:  map[string]string{},
		msgTurn:          map[string]string{},
		turnConversation: map[string]string{},
	}
}

func (c *tolerantClient) PatchTurn(ctx context.Context, in *apiconv.MutableTurn) error {
	if in != nil && in.Has != nil && in.Has.Id {
		if in.Has.ConversationID {
			c.turnConversation[in.Id] = in.ConversationID
		} else if convID := c.turnConversation[in.Id]; convID != "" {
			in.SetConversationID(convID)
		}
	}
	return c.Client.PatchTurn(ctx, in)
}

func (c *tolerantClient) PatchMessage(ctx context.Context, in *apiconv.MutableMessage) error {
	if in != nil && in.Has != nil && in.Has.Id {
		if in.Has.ConversationID {
			c.msgConversation[in.Id] = in.ConversationID
		} else if convID := c.msgConversation[in.Id]; convID != "" {
			in.SetConversationID(convID)
		}
		if in.Has.TurnID && in.TurnID != nil {
			c.msgTurn[in.Id] = *in.TurnID
		} else if turnID := c.msgTurn[in.Id]; turnID != "" {
			in.SetTurnID(turnID)
		}
	}
	return c.Client.PatchMessage(ctx, in)
}

func TestService_PersistTurnFailure_UpdatesSeedMessageStatus(t *testing.T) {
	testCases := []struct {
		name          string
		err           error
		wantTurn      string
		wantMsgStatus string
	}{
		{
			name:          "failed turn marks seed message rejected",
			err:           fmt.Errorf("boom"),
			wantTurn:      "failed",
			wantMsgStatus: "rejected",
		},
		{
			name:          "canceled turn marks seed message cancel",
			err:           context.Canceled,
			wantTurn:      "canceled",
			wantMsgStatus: "cancel",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			client := newTolerantClient()
			svc := NewServiceWithClient(client, nil)

			conv := apiconv.NewConversation()
			conv.SetId("c1")
			require.NoError(t, client.PatchConversations(ctx, conv))

			turn := apiconv.NewTurn()
			turn.SetId("t1")
			turn.SetConversationID("c1")
			turn.SetStatus("queued")
			turn.SetStartedByMessageID("t1")
			require.NoError(t, client.PatchTurn(ctx, turn))

			msg := apiconv.NewMessage()
			msg.SetId("t1")
			msg.SetConversationID("c1")
			msg.SetTurnID("t1")
			msg.SetRole("user")
			msg.SetType("text")
			msg.SetContent("hi")
			msg.SetStatus("pending")
			require.NoError(t, client.PatchMessage(ctx, msg))

			gotErr := svc.persistTurnFailure(ctx, "t1", tc.err)
			require.True(t, errors.Is(gotErr, tc.err), "persistTurnFailure should return original error")

			convOut, err := client.GetConversation(ctx, "c1")
			require.NoError(t, err)
			require.NotNil(t, convOut)
			require.Len(t, convOut.Transcript, 1)
			require.Equal(t, tc.wantTurn, convOut.Transcript[0].Status)

			msgOut, err := client.GetMessage(ctx, "t1")
			require.NoError(t, err)
			require.NotNil(t, msgOut)
			require.NotNil(t, msgOut.Status)
			require.Equal(t, tc.wantMsgStatus, *msgOut.Status)
		})
	}
}
