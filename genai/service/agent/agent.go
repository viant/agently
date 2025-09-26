package agent

import (
	"context"
	"fmt"
)

// ensureAgent populates qi.Agent (using finder when needed) and echoes it on
// qo.Agent for caller convenience.
func (s *Service) ensureAgent(ctx context.Context, qi *QueryInput) error {
	if qi.Agent == nil && qi.AgentName != "" {
		a, err := s.agentFinder.Find(ctx, qi.AgentName)
		if err != nil {
			return fmt.Errorf("failed to load agent: %w", err)
		}
		qi.Agent = a
	}
	if qi.Agent == nil {
		return fmt.Errorf("agent is required")
	}
	return nil
}
