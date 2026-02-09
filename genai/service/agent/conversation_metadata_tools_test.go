package agent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	apiconv "github.com/viant/agently/client/conversation"
)

type convoStub struct {
	conv *apiconv.Conversation
}

func (c *convoStub) GetConversation(ctx context.Context, id string, options ...apiconv.Option) (*apiconv.Conversation, error) {
	return c.conv, nil
}
func (c *convoStub) GetConversations(ctx context.Context, input *apiconv.Input) ([]*apiconv.Conversation, error) {
	return nil, nil
}
func (c *convoStub) PatchConversations(ctx context.Context, conversations *apiconv.MutableConversation) error {
	return nil
}
func (c *convoStub) GetPayload(ctx context.Context, id string) (*apiconv.Payload, error) {
	return nil, nil
}
func (c *convoStub) PatchPayload(ctx context.Context, payload *apiconv.MutablePayload) error {
	return nil
}
func (c *convoStub) PatchMessage(ctx context.Context, message *apiconv.MutableMessage) error {
	return nil
}
func (c *convoStub) GetMessage(ctx context.Context, id string, options ...apiconv.Option) (*apiconv.Message, error) {
	return nil, nil
}
func (c *convoStub) GetMessageByElicitation(ctx context.Context, conversationID, elicitationID string) (*apiconv.Message, error) {
	return nil, nil
}
func (c *convoStub) PatchModelCall(ctx context.Context, modelCall *apiconv.MutableModelCall) error {
	return nil
}
func (c *convoStub) PatchToolCall(ctx context.Context, toolCall *apiconv.MutableToolCall) error {
	return nil
}
func (c *convoStub) PatchTurn(ctx context.Context, turn *apiconv.MutableTurn) error {
	return nil
}
func (c *convoStub) DeleteConversation(ctx context.Context, id string) error {
	return nil
}
func (c *convoStub) DeleteMessage(ctx context.Context, conversationID, messageID string) error {
	return nil
}

func TestEnsureConversation_SkipsMetaToolsWhenAutoAgent(t *testing.T) {
	meta := ConversationMetadata{Tools: []string{"system/exec:execute"}}
	metaBytes, err := json.Marshal(meta)
	require.NoError(t, err)
	now := time.Now()
	agentID := "auto"
	conv := &apiconv.Conversation{
		Id:           "c1",
		AgentId:      &agentID,
		Metadata:     ptrString(string(metaBytes)),
		CreatedAt:    now,
		UpdatedAt:    &now,
		LastActivity: &now,
	}
	svc := &Service{conversation: &convoStub{conv: conv}}

	in := &QueryInput{ConversationID: "c1", AgentID: "chosen-agent"}
	err = svc.ensureConversation(context.Background(), in)
	require.NoError(t, err)
	require.Empty(t, in.ToolsAllowed, "meta tools should be skipped when agent is auto-selected")
}

func TestEnsureConversation_AppliesMetaToolsWhenAgentMatches(t *testing.T) {
	meta := ConversationMetadata{Tools: []string{"system/exec:execute"}}
	metaBytes, err := json.Marshal(meta)
	require.NoError(t, err)
	now := time.Now()
	agentID := "agent-a"
	conv := &apiconv.Conversation{
		Id:           "c1",
		AgentId:      &agentID,
		Metadata:     ptrString(string(metaBytes)),
		CreatedAt:    now,
		UpdatedAt:    &now,
		LastActivity: &now,
	}
	svc := &Service{conversation: &convoStub{conv: conv}}

	in := &QueryInput{ConversationID: "c1", AgentID: "agent-a"}
	err = svc.ensureConversation(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, []string{"system/exec:execute"}, in.ToolsAllowed)
}

func ptrString(s string) *string { return &s }
