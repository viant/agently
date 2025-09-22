package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/viant/agently/genai/agent/plan"
)

func (s *Service) recordAssistantElicitation(ctx context.Context, convID string, messageID string, elic *plan.Elicitation) error {
	// Refine schema for better UX using injected service.
	s.refiner.RefineRequestedSchema(&elic.RequestedSchema)
	// Ensure elicitationId is present for client correlation.
	if strings.TrimSpace(elic.ElicitationId) == "" {
		elic.ElicitationId = uuid.New().String()
	}
	if err := s.elicitation.Record(ctx, convID, "assistant", messageID, elic); err != nil {
		return fmt.Errorf("recordAssistantElicitation: %v", err)
	}
	return nil
}
