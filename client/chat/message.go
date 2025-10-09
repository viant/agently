package chat

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/viant/agently/genai/memory"
)

var ErrInvalidInput = fmt.Errorf("invalid input")

// MessageClient captures the minimal method set needed to persist a message.
// It is satisfied by chat/store.Client to avoid introducing a package cycle.
type MessageClient interface {
	PatchMessage(ctx context.Context, message *MutableMessage) error
}

// AddMessage creates and persists a message attached to the given turn using the provided options.
// It sets sensible defaults: id (uuid), conversation/turn/parent ids from turn, and type "text" unless overridden.
// Returns the message id.
func AddMessage(ctx context.Context, cl MessageClient, turn *memory.TurnMeta, opts ...MessageOption) (string, error) {
	if cl == nil || turn == nil {
		return "", ErrInvalidInput
	}
	m := NewMessage()
	if strings.TrimSpace(turn.ConversationID) != "" {
		m.SetConversationID(turn.ConversationID)
	}
	if strings.TrimSpace(turn.TurnID) != "" {
		m.SetTurnID(turn.TurnID)
	}
	if strings.TrimSpace(turn.ParentMessageID) != "" {
		m.SetParentMessageID(turn.ParentMessageID)
	}
	m.SetType("text")
	for _, opt := range opts {
		if opt != nil {
			opt(m)
		}
	}
	if strings.TrimSpace(m.Id) == "" {
		m.SetId(uuid.New().String())
	}
	if err := cl.PatchMessage(ctx, m); err != nil {
		return "", err
	}
	return m.Id, nil
}
