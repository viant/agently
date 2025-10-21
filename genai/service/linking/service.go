package linking

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/memory"
	convw "github.com/viant/agently/pkg/agently/conversation/write"
)

// Service encapsulates helpers to create child conversations linked to a parent
// turn and to add parent-side link messages. It centralizes conversation
// linkage so both internal and external agent runs can rely on consistent
// behavior.
type Service struct {
	conv apiconv.Client
}

// New returns a new linking Service.
func New(c apiconv.Client) *Service { return &Service{conv: c} }

// CreateLinkedConversation creates a new conversation linked to the provided
// parent turn (by conversation/turn id). When cloneTranscript is true and a
// transcript is provided, it clones the last transcript into the new
// conversation for context.
func (s *Service) CreateLinkedConversation(ctx context.Context, parent memory.TurnMeta, cloneTranscript bool, transcript apiconv.Transcript) (string, error) {
	if s == nil || s.conv == nil {
		return "", fmt.Errorf("linking: conversation client not configured")
	}
	childID := uuid.New().String()
	// Create child conversation and set parent ids
	w := convw.Conversation{Has: &convw.ConversationHas{}}
	w.SetId(childID)
	w.SetVisibility(convw.VisibilityPublic)
	if strings.TrimSpace(parent.ConversationID) != "" {
		w.SetConversationParentId(parent.ConversationID)
	}
	if strings.TrimSpace(parent.TurnID) != "" {
		w.SetConversationParentTurnId(parent.TurnID)
	}
	if err := s.conv.PatchConversations(ctx, (*apiconv.MutableConversation)(&w)); err != nil {
		return "", fmt.Errorf("linking: create conversation failed: %w", err)
	}
	if cloneTranscript && transcript != nil {
		// Clone messages (excluding chain-mode) as a single synthetic turn
		if err := s.cloneMessages(ctx, transcript, childID); err != nil {
			return "", err
		}
	}
	return childID, nil
}

// AddLinkMessage adds an interim message to the parent turn with a linked
// conversation id so UIs and tooling can navigate to the child.
func (s *Service) AddLinkMessage(ctx context.Context, parent memory.TurnMeta, childConversationID, role, actor, mode string) error {
	if s == nil || s.conv == nil {
		return fmt.Errorf("linking: conversation client not configured")
	}
	if strings.TrimSpace(role) == "" {
		role = "assistant"
	}
	if strings.TrimSpace(actor) == "" {
		actor = "link"
	}
	if strings.TrimSpace(mode) == "" {
		mode = "link"
	}
	_, err := apiconv.AddMessage(ctx, s.conv, &parent,
		apiconv.WithId(uuid.New().String()),
		apiconv.WithRole(role),
		apiconv.WithInterim(1),
		apiconv.WithContent(""),
		apiconv.WithCreatedByUserID(actor),
		apiconv.WithMode(mode),
		apiconv.WithLinkedConversationID(childConversationID),
	)
	if err != nil {
		return fmt.Errorf("linking: add link message failed: %w", err)
	}
	return nil
}

// cloneMessages clones the last transcript into a new conversation under a
// synthetic turn, excluding messages with mode == "chain".
func (s *Service) cloneMessages(ctx context.Context, transcript apiconv.Transcript, conversationID string) error {
	if transcript == nil || len(transcript) == 0 {
		return nil
	}
	turnID := uuid.New().String()
	turn := memory.TurnMeta{ParentMessageID: turnID, TurnID: turnID, ConversationID: conversationID}
	mt := apiconv.NewTurn()
	mt.SetId(turn.TurnID)
	mt.SetConversationID(turn.ConversationID)
	mt.SetStatus("running")
	if err := s.conv.PatchTurn(ctx, mt); err != nil {
		return fmt.Errorf("linking: start synthetic turn failed: %w", err)
	}
	last := transcript[0]
	for _, m := range last.GetMessages() {
		if m.Mode != nil && *m.Mode == "chain" {
			continue
		}
		mut := m.NewMutable()
		mut.SetId(uuid.New().String())
		mut.SetTurnID(turn.TurnID)
		mut.SetConversationID(turn.ConversationID)
		mut.SetParentMessageID(turn.ParentMessageID)
		if err := s.conv.PatchMessage(ctx, mut); err != nil {
			return fmt.Errorf("linking: clone message failed: %w", err)
		}
	}
	return nil
}
