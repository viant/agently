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

type fakeCancelRegistry struct {
	canceledConversationID []string
	cancelReturn           bool
}

func (f *fakeCancelRegistry) Register(string, string, context.CancelFunc) {}
func (f *fakeCancelRegistry) Complete(string, string, context.CancelFunc) {}
func (f *fakeCancelRegistry) CancelTurn(string) bool                      { return false }
func (f *fakeCancelRegistry) CancelConversation(conversationID string) bool {
	f.canceledConversationID = append(f.canceledConversationID, conversationID)
	return f.cancelReturn
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

func TestService_TerminateConversation_CancelsAndMarksConversation(t *testing.T) {
	ctx := context.Background()
	client := newTolerantClient()
	svc := NewServiceWithClient(client, nil)
	reg := &fakeCancelRegistry{cancelReturn: true}
	svc.reg = reg

	conv := apiconv.NewConversation()
	conv.SetId("c1")
	conv.SetStatus("running")
	require.NoError(t, client.PatchConversations(ctx, conv))

	turn := apiconv.NewTurn()
	turn.SetId("t1")
	turn.SetConversationID("c1")
	turn.SetStatus("running")
	require.NoError(t, client.PatchTurn(ctx, turn))

	userMsg := apiconv.NewMessage()
	userMsg.SetId("u1")
	userMsg.SetConversationID("c1")
	userMsg.SetTurnID("t1")
	userMsg.SetRole("user")
	userMsg.SetType("text")
	userMsg.SetContent("hi")
	require.NoError(t, client.PatchMessage(ctx, userMsg))

	assistantMsg := apiconv.NewMessage()
	assistantMsg.SetId("a1")
	assistantMsg.SetConversationID("c1")
	assistantMsg.SetTurnID("t1")
	assistantMsg.SetRole("assistant")
	assistantMsg.SetType("text")
	assistantMsg.SetContent("working")
	assistantMsg.SetStatus("open")
	require.NoError(t, client.PatchMessage(ctx, assistantMsg))

	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()

	canceled, err := svc.TerminateConversation(canceledCtx, "c1")
	require.NoError(t, err)
	require.True(t, canceled)
	require.Equal(t, []string{"c1"}, reg.canceledConversationID)

	convOut, err := client.GetConversation(ctx, "c1")
	require.NoError(t, err)
	require.NotNil(t, convOut)
	require.NotNil(t, convOut.Status)
	require.Equal(t, "canceled", *convOut.Status)
	require.Len(t, convOut.Transcript, 1)
	require.Equal(t, "canceled", convOut.Transcript[0].Status)

	msgOut, err := client.GetMessage(ctx, "a1")
	require.NoError(t, err)
	require.NotNil(t, msgOut)
	require.NotNil(t, msgOut.Status)
	require.Equal(t, "cancel", *msgOut.Status)
}

func TestService_TerminateConversation_CancelsLinkedChildrenWithoutLiveRegistryState(t *testing.T) {
	ctx := context.Background()
	client := newTolerantClient()
	svc := NewServiceWithClient(client, nil)
	reg := &fakeCancelRegistry{}
	svc.reg = reg

	parent := apiconv.NewConversation()
	parent.SetId("parent")
	parent.SetStatus("running")
	require.NoError(t, client.PatchConversations(ctx, parent))

	parentTurn := apiconv.NewTurn()
	parentTurn.SetId("parent-turn")
	parentTurn.SetConversationID("parent")
	parentTurn.SetStatus("running")
	require.NoError(t, client.PatchTurn(ctx, parentTurn))

	parentMsg := apiconv.NewMessage()
	parentMsg.SetId("parent-assistant")
	parentMsg.SetConversationID("parent")
	parentMsg.SetTurnID("parent-turn")
	parentMsg.SetRole("assistant")
	parentMsg.SetType("text")
	parentMsg.SetContent("working")
	parentMsg.SetStatus("open")
	require.NoError(t, client.PatchMessage(ctx, parentMsg))

	child := apiconv.NewConversation()
	child.SetId("child")
	child.SetStatus("running")
	child.SetConversationParentId("parent")
	child.SetConversationParentTurnId("parent-turn")
	require.NoError(t, client.PatchConversations(ctx, child))

	childTurn := apiconv.NewTurn()
	childTurn.SetId("child-turn")
	childTurn.SetConversationID("child")
	childTurn.SetStatus("running")
	require.NoError(t, client.PatchTurn(ctx, childTurn))

	childAssistant := apiconv.NewMessage()
	childAssistant.SetId("child-assistant")
	childAssistant.SetConversationID("child")
	childAssistant.SetTurnID("child-turn")
	childAssistant.SetRole("assistant")
	childAssistant.SetType("text")
	childAssistant.SetContent("still working")
	childAssistant.SetStatus("open")
	require.NoError(t, client.PatchMessage(ctx, childAssistant))

	childToolMsg := apiconv.NewMessage()
	childToolMsg.SetId("child-tool")
	childToolMsg.SetConversationID("child")
	childToolMsg.SetTurnID("child-turn")
	childToolMsg.SetRole("tool")
	childToolMsg.SetType("tool_op")
	childToolMsg.SetContent("tool running")
	childToolMsg.SetStatus("open")
	require.NoError(t, client.PatchMessage(ctx, childToolMsg))

	childTool := apiconv.NewToolCall()
	childTool.SetMessageID("child-tool")
	childTool.SetOpID("op-child")
	childTool.SetAttempt(1)
	childTool.SetToolName("llm_agents-run")
	childTool.SetToolKind("llm")
	childTool.SetStatus("running")
	require.NoError(t, client.PatchToolCall(ctx, childTool))

	grandchild := apiconv.NewConversation()
	grandchild.SetId("grandchild")
	grandchild.SetStatus("running")
	grandchild.SetConversationParentId("child")
	grandchild.SetConversationParentTurnId("child-turn")
	require.NoError(t, client.PatchConversations(ctx, grandchild))

	grandchildTurn := apiconv.NewTurn()
	grandchildTurn.SetId("grandchild-turn")
	grandchildTurn.SetConversationID("grandchild")
	grandchildTurn.SetStatus("running")
	require.NoError(t, client.PatchTurn(ctx, grandchildTurn))

	grandchildAssistant := apiconv.NewMessage()
	grandchildAssistant.SetId("grandchild-assistant")
	grandchildAssistant.SetConversationID("grandchild")
	grandchildAssistant.SetTurnID("grandchild-turn")
	grandchildAssistant.SetRole("assistant")
	grandchildAssistant.SetType("text")
	grandchildAssistant.SetContent("grandchild working")
	grandchildAssistant.SetStatus("open")
	require.NoError(t, client.PatchMessage(ctx, grandchildAssistant))

	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()

	canceled, err := svc.TerminateConversation(canceledCtx, "parent")
	require.NoError(t, err)
	require.False(t, canceled)
	require.Equal(t, []string{"parent", "child", "grandchild"}, reg.canceledConversationID)

	parentOut, err := client.GetConversation(ctx, "parent", apiconv.WithIncludeToolCall(true))
	require.NoError(t, err)
	require.NotNil(t, parentOut)
	require.NotNil(t, parentOut.Status)
	require.Equal(t, "canceled", *parentOut.Status)
	require.Len(t, parentOut.Transcript, 1)
	require.Equal(t, "canceled", parentOut.Transcript[0].Status)

	parentMsgOut, err := client.GetMessage(ctx, "parent-assistant")
	require.NoError(t, err)
	require.NotNil(t, parentMsgOut)
	require.NotNil(t, parentMsgOut.Status)
	require.Equal(t, "cancel", *parentMsgOut.Status)

	childOut, err := client.GetConversation(ctx, "child", apiconv.WithIncludeToolCall(true))
	require.NoError(t, err)
	require.NotNil(t, childOut)
	require.NotNil(t, childOut.Status)
	require.Equal(t, "canceled", *childOut.Status)
	require.Len(t, childOut.Transcript, 1)
	require.Equal(t, "canceled", childOut.Transcript[0].Status)

	childAssistantOut, err := client.GetMessage(ctx, "child-assistant")
	require.NoError(t, err)
	require.NotNil(t, childAssistantOut)
	require.NotNil(t, childAssistantOut.Status)
	require.Equal(t, "cancel", *childAssistantOut.Status)

	childToolOut, err := client.GetMessage(ctx, "child-tool")
	require.NoError(t, err)
	require.NotNil(t, childToolOut)
	require.NotNil(t, childToolOut.ToolCall)
	require.Equal(t, "canceled", childToolOut.ToolCall.Status)
	require.NotNil(t, childToolOut.ToolCall.CompletedAt)
	require.NotNil(t, childToolOut.ToolCall.ErrorMessage)
	require.Equal(t, "conversation terminated", *childToolOut.ToolCall.ErrorMessage)

	grandchildOut, err := client.GetConversation(ctx, "grandchild", apiconv.WithIncludeToolCall(true))
	require.NoError(t, err)
	require.NotNil(t, grandchildOut)
	require.NotNil(t, grandchildOut.Status)
	require.Equal(t, "canceled", *grandchildOut.Status)
	require.Len(t, grandchildOut.Transcript, 1)
	require.Equal(t, "canceled", grandchildOut.Transcript[0].Status)

	grandchildAssistantOut, err := client.GetMessage(ctx, "grandchild-assistant")
	require.NoError(t, err)
	require.NotNil(t, grandchildAssistantOut)
	require.NotNil(t, grandchildAssistantOut.Status)
	require.Equal(t, "cancel", *grandchildAssistantOut.Status)
}

func TestService_TerminateConversation_PreservesSucceededDescendants(t *testing.T) {
	ctx := context.Background()
	client := newTolerantClient()
	svc := NewServiceWithClient(client, nil)
	reg := &fakeCancelRegistry{}
	svc.reg = reg

	parent := apiconv.NewConversation()
	parent.SetId("parent")
	parent.SetStatus("running")
	require.NoError(t, client.PatchConversations(ctx, parent))

	parentTurn := apiconv.NewTurn()
	parentTurn.SetId("parent-turn")
	parentTurn.SetConversationID("parent")
	parentTurn.SetStatus("running")
	require.NoError(t, client.PatchTurn(ctx, parentTurn))

	parentAssistant := apiconv.NewMessage()
	parentAssistant.SetId("parent-assistant")
	parentAssistant.SetConversationID("parent")
	parentAssistant.SetTurnID("parent-turn")
	parentAssistant.SetRole("assistant")
	parentAssistant.SetType("text")
	parentAssistant.SetContent("still running")
	parentAssistant.SetStatus("open")
	require.NoError(t, client.PatchMessage(ctx, parentAssistant))

	child := apiconv.NewConversation()
	child.SetId("child")
	child.SetStatus("succeeded")
	child.SetConversationParentId("parent")
	child.SetConversationParentTurnId("parent-turn")
	require.NoError(t, client.PatchConversations(ctx, child))

	childTurn := apiconv.NewTurn()
	childTurn.SetId("child-turn")
	childTurn.SetConversationID("child")
	childTurn.SetStatus("succeeded")
	require.NoError(t, client.PatchTurn(ctx, childTurn))

	childAssistant := apiconv.NewMessage()
	childAssistant.SetId("child-assistant")
	childAssistant.SetConversationID("child")
	childAssistant.SetTurnID("child-turn")
	childAssistant.SetRole("assistant")
	childAssistant.SetType("text")
	childAssistant.SetContent("child result")
	childAssistant.SetStatus("completed")
	require.NoError(t, client.PatchMessage(ctx, childAssistant))

	childToolMsg := apiconv.NewMessage()
	childToolMsg.SetId("child-tool")
	childToolMsg.SetConversationID("child")
	childToolMsg.SetTurnID("child-turn")
	childToolMsg.SetRole("tool")
	childToolMsg.SetType("tool_op")
	childToolMsg.SetContent("done")
	childToolMsg.SetStatus("completed")
	require.NoError(t, client.PatchMessage(ctx, childToolMsg))

	childTool := apiconv.NewToolCall()
	childTool.SetMessageID("child-tool")
	childTool.SetOpID("op-child")
	childTool.SetAttempt(1)
	childTool.SetToolName("llm_agents-run")
	childTool.SetToolKind("llm")
	childTool.SetStatus("completed")
	require.NoError(t, client.PatchToolCall(ctx, childTool))

	grandchild := apiconv.NewConversation()
	grandchild.SetId("grandchild")
	grandchild.SetStatus("succeeded")
	grandchild.SetConversationParentId("child")
	grandchild.SetConversationParentTurnId("child-turn")
	require.NoError(t, client.PatchConversations(ctx, grandchild))

	grandchildTurn := apiconv.NewTurn()
	grandchildTurn.SetId("grandchild-turn")
	grandchildTurn.SetConversationID("grandchild")
	grandchildTurn.SetStatus("succeeded")
	require.NoError(t, client.PatchTurn(ctx, grandchildTurn))

	grandchildAssistant := apiconv.NewMessage()
	grandchildAssistant.SetId("grandchild-assistant")
	grandchildAssistant.SetConversationID("grandchild")
	grandchildAssistant.SetTurnID("grandchild-turn")
	grandchildAssistant.SetRole("assistant")
	grandchildAssistant.SetType("text")
	grandchildAssistant.SetContent("grandchild result")
	grandchildAssistant.SetStatus("completed")
	require.NoError(t, client.PatchMessage(ctx, grandchildAssistant))

	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()

	canceled, err := svc.TerminateConversation(canceledCtx, "parent")
	require.NoError(t, err)
	require.False(t, canceled)
	require.Equal(t, []string{"parent", "child", "grandchild"}, reg.canceledConversationID)

	parentOut, err := client.GetConversation(ctx, "parent", apiconv.WithIncludeToolCall(true))
	require.NoError(t, err)
	require.NotNil(t, parentOut)
	require.NotNil(t, parentOut.Status)
	require.Equal(t, "canceled", *parentOut.Status)
	require.Len(t, parentOut.Transcript, 1)
	require.Equal(t, "canceled", parentOut.Transcript[0].Status)

	parentAssistantOut, err := client.GetMessage(ctx, "parent-assistant")
	require.NoError(t, err)
	require.NotNil(t, parentAssistantOut)
	require.NotNil(t, parentAssistantOut.Status)
	require.Equal(t, "cancel", *parentAssistantOut.Status)

	childOut, err := client.GetConversation(ctx, "child", apiconv.WithIncludeToolCall(true))
	require.NoError(t, err)
	require.NotNil(t, childOut)
	require.NotNil(t, childOut.Status)
	require.Equal(t, "succeeded", *childOut.Status)
	require.Len(t, childOut.Transcript, 1)
	require.Equal(t, "succeeded", childOut.Transcript[0].Status)

	childAssistantOut, err := client.GetMessage(ctx, "child-assistant")
	require.NoError(t, err)
	require.NotNil(t, childAssistantOut)
	require.NotNil(t, childAssistantOut.Status)
	require.Equal(t, "completed", *childAssistantOut.Status)

	childToolOut, err := client.GetMessage(ctx, "child-tool")
	require.NoError(t, err)
	require.NotNil(t, childToolOut)
	require.NotNil(t, childToolOut.ToolCall)
	require.Equal(t, "completed", childToolOut.ToolCall.Status)

	grandchildOut, err := client.GetConversation(ctx, "grandchild", apiconv.WithIncludeToolCall(true))
	require.NoError(t, err)
	require.NotNil(t, grandchildOut)
	require.NotNil(t, grandchildOut.Status)
	require.Equal(t, "succeeded", *grandchildOut.Status)
	require.Len(t, grandchildOut.Transcript, 1)
	require.Equal(t, "succeeded", grandchildOut.Transcript[0].Status)

	grandchildAssistantOut, err := client.GetMessage(ctx, "grandchild-assistant")
	require.NoError(t, err)
	require.NotNil(t, grandchildAssistantOut)
	require.NotNil(t, grandchildAssistantOut.Status)
	require.Equal(t, "completed", *grandchildAssistantOut.Status)
}

func TestIsScheduledConversation(t *testing.T) {
	one := 1
	scheduleID := "sched-1"
	runID := "run-1"

	testCases := []struct {
		name string
		conv *apiconv.Conversation
		want bool
	}{
		{
			name: "scheduled flag",
			conv: &apiconv.Conversation{Scheduled: &one},
			want: true,
		},
		{
			name: "schedule id only",
			conv: &apiconv.Conversation{ScheduleId: &scheduleID},
			want: true,
		},
		{
			name: "schedule run id only",
			conv: &apiconv.Conversation{ScheduleRunId: &runID},
			want: true,
		},
		{
			name: "manual conversation",
			conv: &apiconv.Conversation{},
			want: false,
		},
		{
			name: "nil conversation",
			conv: nil,
			want: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isScheduledConversation(tc.conv))
		})
	}
}
