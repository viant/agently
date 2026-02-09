package elicitation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/elicitation/router"
	"github.com/viant/agently/genai/memory"
)

type patchCall struct {
	id             string
	conversationID string
	turnID         string
	seqProvided    bool
	seq            int
}

type seqRecordingConv struct {
	apiconv.Client

	childConversationID string
	conversations       map[string]*apiconv.Conversation

	byID   map[string]*apiconv.Message
	byElic map[string]*apiconv.Message

	patches []patchCall
}

func newSeqRecordingConv(childConversationID string) *seqRecordingConv {
	return &seqRecordingConv{
		childConversationID: childConversationID,
		conversations:       map[string]*apiconv.Conversation{},
		byID:                map[string]*apiconv.Message{},
		byElic:              map[string]*apiconv.Message{},
	}
}

func (f *seqRecordingConv) GetConversation(ctx context.Context, id string, options ...apiconv.Option) (*apiconv.Conversation, error) {
	if c, ok := f.conversations[id]; ok {
		return c, nil
	}
	return &apiconv.Conversation{Id: id}, nil
}

func (f *seqRecordingConv) PatchConversations(ctx context.Context, conversations *apiconv.MutableConversation) error {
	return nil
}

func (f *seqRecordingConv) PatchPayload(ctx context.Context, payload *apiconv.MutablePayload) error {
	return nil
}

func (f *seqRecordingConv) GetMessageByElicitation(ctx context.Context, conversationID, elicitationID string) (*apiconv.Message, error) {
	return f.byElic[conversationID+"/"+elicitationID], nil
}

func (f *seqRecordingConv) GetMessage(ctx context.Context, id string, _ ...apiconv.Option) (*apiconv.Message, error) {
	return f.byID[id], nil
}

func (f *seqRecordingConv) DeleteMessage(ctx context.Context, conversationID, messageID string) error {
	return nil
}

func (f *seqRecordingConv) PatchMessage(ctx context.Context, m *apiconv.MutableMessage) error {
	if m == nil {
		return nil
	}
	var turnID string
	if m.TurnID != nil {
		turnID = *m.TurnID
	}
	call := patchCall{
		id:             m.Id,
		conversationID: m.ConversationID,
		turnID:         turnID,
		seqProvided:    m.Sequence != nil,
	}
	if m.Sequence != nil {
		call.seq = *m.Sequence
	}
	f.patches = append(f.patches, call)

	// Simulate DB sequence assignment for the initial insert in the child conversation.
	if _, exists := f.byID[m.Id]; !exists {
		if m.ConversationID == f.childConversationID && m.Sequence == nil && m.TurnID != nil && *m.TurnID != "" {
			m.SetSequence(5)
		}
	}

	// Store a minimal read view for later lookups.
	if m.Id != "" {
		mv := &apiconv.Message{Id: m.Id, ConversationId: m.ConversationID, Role: m.Role, Type: m.Type}
		if m.Content != "" {
			cpy := m.Content
			mv.Content = &cpy
		}
		if m.TurnID != nil {
			mv.TurnId = m.TurnID
		}
		f.byID[m.Id] = mv
		if m.ElicitationID != nil && *m.ElicitationID != "" {
			f.byElic[m.ConversationID+"/"+*m.ElicitationID] = mv
		}
	}
	return nil
}

func TestElicit_RootDuplicateDoesNotReuseSequence(t *testing.T) {
	childID := "conv-child"
	parentID := "conv-parent"
	rootID := "conv-root"
	childTurnID := "turn-child"
	parentTurnID := "turn-parent"

	fake := newSeqRecordingConv(childID)
	fake.conversations[childID] = &apiconv.Conversation{Id: childID, ConversationParentId: &parentID}
	fake.conversations[parentID] = &apiconv.Conversation{Id: parentID, ConversationParentId: &rootID, LastTurnId: &parentTurnID}
	fake.conversations[rootID] = &apiconv.Conversation{Id: rootID}

	r := router.New()
	srv := New(fake, nil, r, func() Awaiter { return acceptNoPayloadAwaiter{} })

	turn := &memory.TurnMeta{ConversationID: childID, TurnID: childTurnID}
	_, _, _, err := srv.Elicit(context.Background(), turn, "assistant", &plan.Elicitation{})
	assert.NoError(t, err)

	found := false
	for _, call := range fake.patches {
		if call.conversationID == parentID && call.turnID == parentTurnID && call.id != "" {
			found = true
			assert.False(t, call.seqProvided, "root-duplicated message must not provide sequence (turn_id, sequence is unique)")
		}
	}
	assert.True(t, found, "expected an additional message to be patched into the parent/root conversation")
}
