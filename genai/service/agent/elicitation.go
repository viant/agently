package agent

import (
	"context"
	"strings"

	"github.com/google/uuid"
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/io/elicitation/refiner"
	"github.com/viant/agently/genai/memory"
)

func (s *Service) recordAssistantElicitation(ctx context.Context, convID string, messageID string, elic *plan.Elicitation) error {
	if elic == nil {
		return nil
	}

	// Refine schema for better UX.
	refiner.Refine(&elic.RequestedSchema)
	// Ensure elicitationId is present for client correlation.
	if strings.TrimSpace(elic.ElicitationId) == "" {
		elic.ElicitationId = uuid.New().String()
	}

	// Persist elicitation assistant message via conversation client
	m := apiconv.NewMessage()
	m.SetId(uuid.New().String())
	m.SetConversationID(convID)
	if turn, ok := memory.TurnMetaFromContext(ctx); ok && strings.TrimSpace(turn.TurnID) != "" {
		m.SetTurnID(turn.TurnID)
	}
	m.SetParentMessageID(messageID)
	m.SetRole("assistant")
	m.SetType("text")
	if strings.TrimSpace(elic.Message) != "" {
		m.SetContent(elic.Message)
	}
	// Elicitation is serialized in message content above
	return s.convClient.PatchMessage(ctx, m)
}
