package chat

import (
	"context"
	"strings"

	"github.com/viant/agently/genai/agent"
)

func (s *Service) conversationAgentID(ctx context.Context, conversationID string) (string, error) {
	if s == nil || s.convClient == nil || strings.TrimSpace(conversationID) == "" {
		return "", nil
	}
	cv, err := s.convClient.GetConversation(ctx, conversationID)
	if err != nil {
		return "", err
	}
	if cv == nil || cv.AgentId == nil {
		return "", nil
	}
	return strings.TrimSpace(*cv.AgentId), nil
}

func (s *Service) agentCatalogSnapshot() []*agent.Agent {
	if s == nil || s.agentFinder == nil {
		return nil
	}
	if c, ok := s.agentFinder.(agentCatalog); ok {
		return c.All()
	}
	return nil
}

func (s *Service) resolveAgentIDForTurn(ctx context.Context, conversationID string, reqAgent string, query string) (string, bool, string, error) {
	conversationAgent, err := s.conversationAgentID(ctx, conversationID)
	if err != nil {
		return "", false, "", err
	}
	defaultAgent := ""
	if s != nil && s.defaults != nil {
		defaultAgent = strings.TrimSpace(s.defaults.Agent)
	}

	reqAgent = strings.TrimSpace(reqAgent)
	autoRequested := isAutoAgentRef(reqAgent) || (reqAgent == "" && isAutoAgentRef(defaultAgent))
	if autoRequested {
		if selected, err := s.classifyAgentIDWithLLM(ctx, conversationID, query, s.agentCatalogSnapshot()); err != nil {
			return "", true, "", err
		} else if strings.TrimSpace(selected) != "" {
			trimmed := strings.TrimSpace(selected)
			return trimmed, true, "", nil
		}
	}

	agentID, autoSelected, err := resolveAgentID(reqAgent, conversationAgent, defaultAgent, query, s.agentCatalogSnapshot())
	return agentID, autoSelected, "", err
}
