package agent

import (
	"context"
	"encoding/json"
	"fmt"
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
	msg := apiconv.NewMessage()
	msg.SetId(uuid.New().String())
	msg.SetConversationID(convID)
	if turn, ok := memory.TurnMetaFromContext(ctx); ok && strings.TrimSpace(turn.TurnID) != "" {
		msg.SetTurnID(turn.TurnID)
	}
	msg.SetElicitationID(elic.ElicitationId)
	msg.SetParentMessageID(messageID)
	msg.SetRole("assistant")
	msg.SetType("text")
	raw, err := json.Marshal(elic)
	if err != nil {
		return fmt.Errorf("recordAssistantElicitation: failed to marshal elic: %v", err)
	}
	if len(raw) > 0 {
		msg.SetContent(string(raw))
	} else {
		msg.SetContent(msg.Content)
	}

	// Elicitation is serialized in message content above
	err = s.convClient.PatchMessage(ctx, msg)
	if err != nil {
		return fmt.Errorf("recordAssistantElicitation: failed to patch message: %v", err)
	}
	return nil
}
