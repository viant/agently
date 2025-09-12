package agent

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
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

	msg := memory.Message{
		ID:             uuid.New().String(),
		ParentID:       messageID,
		ConversationID: convID,
		Role:           "assistant",
		Content:        elic.Message,
		Elicitation:    elic,
		CreatedAt:      time.Now(),
	}
	return s.recorder.RecordMessage(ctx, msg)
}
