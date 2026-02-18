package conversation

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/viant/agently/genai/memory"
	convw "github.com/viant/agently/pkg/agently/conversation/write"
)

// AddMessage creates and persists a message attached to the given turn using the provided options.
// It sets sensible defaults: id (uuid), conversation/turn/parent ids from turn, and type "text" unless overridden.
// Returns the message id.
func AddMessage(ctx context.Context, cl Client, turn *memory.TurnMeta, opts ...MessageOption) (*MutableMessage, error) {
	if cl == nil || turn == nil {
		errorf("AddMessage invalid input cl_nil=%v turn_nil=%v", cl == nil, turn == nil)
		return nil, ErrInvalidInput
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

	debugf("AddMessage start id=%q convo=%q turn=%q parent=%q role=%q type=%q status=%q interim=%v", strings.TrimSpace(m.Id), strings.TrimSpace(m.ConversationID), strings.TrimSpace(valueOrEmptyStr(m.TurnID)), strings.TrimSpace(valueOrEmptyStr(m.ParentMessageID)), strings.TrimSpace(m.Role), strings.TrimSpace(m.Type), strings.TrimSpace(m.Status), valueOrZero(m.Interim))
	// set conversation status to "" (active) if this is a non-interim assistant message and conversation not in summary status
	if (m.Interim == nil || *m.Interim == 0) && m.Role == "assistant" && m.Status != "summary" {
		status := ""
		patch := &convw.Conversation{Has: &convw.ConversationHas{}}
		patch.SetId(m.ConversationID)
		patch.SetStatus(status)
		if err := cl.PatchConversations(ctx, patch); err != nil {
			errorf("AddMessage patch conversation status error id=%q convo=%q err=%v", strings.TrimSpace(m.Id), strings.TrimSpace(m.ConversationID), err)
			return nil, fmt.Errorf("failed to update conversation status: %w", err)
		}
		debugf("AddMessage patched conversation status id=%q convo=%q status=%q", strings.TrimSpace(m.Id), strings.TrimSpace(m.ConversationID), status)
	}

	if err := cl.PatchMessage(ctx, m); err != nil {
		errorf("AddMessage patch message error id=%q convo=%q err=%v", strings.TrimSpace(m.Id), strings.TrimSpace(m.ConversationID), err)
		return nil, err
	}
	debugf("AddMessage ok id=%q convo=%q", strings.TrimSpace(m.Id), strings.TrimSpace(m.ConversationID))
	return m, nil
}

// ErrInvalidInput is returned when required inputs are missing.
var ErrInvalidInput = errInvalidInput{}

type errInvalidInput struct{}

func (e errInvalidInput) Error() string { return "invalid input" }

func valueOrZero(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func valueOrEmptyStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
