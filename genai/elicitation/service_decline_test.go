package elicitation

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/elicitation/router"
	"github.com/viant/agently/genai/memory"
)

// fakeConv is a lightweight in-memory conversation client for tests.
type fakeConv struct {
	apiconv.Client
	byID   map[string]*apiconv.Message
	byElic map[string]*apiconv.Message // convID/elicID -> message

	lastPatchedStatus string
	lastUserContent   string
	lastUserRaw       string
}

func newFakeConv() *fakeConv {
	return &fakeConv{byID: map[string]*apiconv.Message{}, byElic: map[string]*apiconv.Message{}}
}

func (f *fakeConv) GetMessageByElicitation(ctx context.Context, conversationID, elicitationID string) (*apiconv.Message, error) {
	return f.byElic[conversationID+"/"+elicitationID], nil
}
func (f *fakeConv) PatchMessage(ctx context.Context, m *apiconv.MutableMessage) error {
	if m == nil {
		return nil
	}
	if m.Status != "" {
		f.lastPatchedStatus = m.Status
	}
	if m.Content != "" && m.Role == "user" {
		f.lastUserContent = m.Content
		if m.RawContent != nil {
			f.lastUserRaw = *m.RawContent
		}
	}
	// store by ID for GetMessage
	if m.Id != "" {
		// synthesize read view
		mm := &apiconv.Message{Id: m.Id, ConversationId: m.ConversationID, Role: m.Role, Type: m.Type}
		if m.Content != "" {
			cpy := m.Content
			mm.Content = &cpy
		}
		if m.TurnID != nil {
			mm.TurnId = m.TurnID
		}
		if m.ParentMessageID != nil {
			mm.ParentMessageId = m.ParentMessageID
		}
		f.byID[m.Id] = mm
	}
	return nil
}
func (f *fakeConv) GetMessage(ctx context.Context, id string, _ ...apiconv.Option) (*apiconv.Message, error) {
	return f.byID[id], nil
}
func (f *fakeConv) PatchPayload(ctx context.Context, _ *apiconv.MutablePayload) error { return nil }
func (f *fakeConv) PatchConversations(ctx context.Context, _ *apiconv.MutableConversation) error {
	return nil
}
func (f *fakeConv) DeleteMessage(ctx context.Context, conversationID, messageID string) error {
	return nil
}
func (f *fakeConv) GetConversation(ctx context.Context, id string, options ...apiconv.Option) (*apiconv.Conversation, error) {
	return &apiconv.Conversation{Id: id}, nil
}

type acceptNoPayloadAwaiter struct{}

func (acceptNoPayloadAwaiter) AwaitElicitation(ctx context.Context, p *plan.Elicitation) (*plan.ElicitResult, error) {
	return &plan.ElicitResult{Action: plan.ElicitResultActionAccept, Payload: nil}, nil
}

func TestWait_AcceptWithoutPayloadDoesNotMarkDeclined(t *testing.T) {
	convID := "conv-1"
	elicID := "elic-1"

	// seed assistant message linked to elicitation id
	fake := newFakeConv()
	contentBytes, _ := json.Marshal(map[string]any{"requestedSchema": map[string]any{"type": "object"}})
	content := string(contentBytes)
	turnID := "turn-1"
	parentID := "msg-parent"
	msg := &apiconv.Message{Id: "msg-1", ConversationId: convID, Role: "assistant", Content: &content}
	msg.TurnId = &turnID
	msg.ParentMessageId = &parentID
	fake.byElic[convID+"/"+elicID] = msg
	fake.byID[msg.Id] = msg

	r := router.New()
	srv := New(fake, nil, r, func() Awaiter { return acceptNoPayloadAwaiter{} })

	act, payload, err := srv.Wait(context.Background(), convID, elicID)
	assert.NoError(t, err)
	assert.EqualValues(t, "accept", act)
	assert.Nil(t, payload)
	// Ensure we did not mark as rejected due to missing payload
	assert.EqualValues(t, "", fake.lastPatchedStatus)
}

func TestResolve_DeclineWithReasonAddsUserMessage(t *testing.T) {
	convID := "conv-2"
	elicID := "elic-2"

	fake := newFakeConv()
	// Seed assistant message for lookup and to attach parent linkage
	content := "{}"
	turnID := "turn-2"
	parentID := "msg-parent-2"
	msg := &apiconv.Message{Id: "msg-2", ConversationId: convID, Role: "assistant", Content: &content}
	msg.TurnId = &turnID
	msg.ParentMessageId = &parentID
	fake.byElic[convID+"/"+elicID] = msg
	fake.byID[msg.Id] = msg
	// parent message exists so UpdateStatus can optionally delete it
	fake.byID[parentID] = &apiconv.Message{Id: parentID, ConversationId: convID}

	r := router.New()
	srv := New(fake, nil, r, nil)

	err := srv.Resolve(context.Background(), convID, elicID, "decline", nil, "not relevant")
	assert.NoError(t, err)
	assert.EqualValues(t, "rejected", fake.lastPatchedStatus)
	// user message captured as JSON string content
	assert.Contains(t, fake.lastUserContent, "declineReason")
}

func TestAddUserResponseMessageStoresRawContent(t *testing.T) {
	fake := newFakeConv()
	srv := &Service{client: fake}
	turn := &memory.TurnMeta{ConversationID: "c-raw", TurnID: "t-raw"}
	payload := map[string]interface{}{"field": "value"}
	assert.NoError(t, srv.AddUserResponseMessage(context.Background(), turn, payload))
	assert.Contains(t, fake.lastUserContent, "field")
	assert.EqualValues(t, fake.lastUserContent, fake.lastUserRaw)
}
