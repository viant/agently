package conversation

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/viant/agently/genai/memory"
)

// AddMessage creates and persists a message attached to the given turn using the provided options.
// It sets sensible defaults: id (uuid), conversation/turn/parent ids from turn, and type "text" unless overridden.
// Returns the message id.
func AddMessage(ctx context.Context, cl Client, turn *memory.TurnMeta, opts ...MessageOption) (string, error) {
	if cl == nil || turn == nil {
		return "", ErrInvalidInput
	}
	m := NewMessage()
	// Defaults from turn
	if strings.TrimSpace(turn.ConversationID) != "" {
		m.SetConversationID(turn.ConversationID)
	}
	if strings.TrimSpace(turn.TurnID) != "" {
		m.SetTurnID(turn.TurnID)
	}
	if strings.TrimSpace(turn.ParentMessageID) != "" {
		m.SetParentMessageID(turn.ParentMessageID)
	}
	// Default type
	m.SetType("text")
	// Apply options (can override defaults)
	for _, opt := range opts {
		if opt != nil {
			opt(m)
		}
	}
	// Ensure id present
	if strings.TrimSpace(m.Id) == "" {
		m.SetId(uuid.New().String())
	}
	if err := cl.PatchMessage(ctx, m); err != nil {
		return "", err
	}
	return m.Id, nil
}

// ErrInvalidInput is returned when required inputs are missing.
var ErrInvalidInput = errInvalidInput{}

type errInvalidInput struct{}

func (e errInvalidInput) Error() string { return "invalid input" }
