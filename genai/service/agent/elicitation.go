package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/viant/agently/genai/agent/plan"
)

func (s *Service) recordAssistantElicitation(ctx context.Context, convID string, messageID string, elic *plan.Elicitation) error {
	if elic == nil {
		return nil
	}

	// Refine schema for better UX using injected service.
	if s.refinerSvc != nil {
		s.refinerSvc.RefineRequestedSchema(&elic.RequestedSchema)
	}
	// Ensure elicitationId is present for client correlation.
	if strings.TrimSpace(elic.ElicitationId) == "" {
		elic.ElicitationId = uuid.New().String()
	}

	// Persist via shared utility as type=elicitation (assistant role)
	if s.elicition == nil {
		return fmt.Errorf("elicitation service not initialised")
	}
	if err := s.elicition.Record(ctx, convID, "assistant", messageID, elic); err != nil {
		return fmt.Errorf("recordAssistantElicitation: %v", err)
	}
	return nil
}
