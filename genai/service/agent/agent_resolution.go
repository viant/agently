package agent

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	apiconv "github.com/viant/agently/client/conversation"
	agentmdl "github.com/viant/agently/genai/agent"
)

type agentCatalog interface {
	All() []*agentmdl.Agent
}

func isAutoAgentRef(agentRef string) bool {
	switch strings.ToLower(strings.TrimSpace(agentRef)) {
	case "", "agent_id", "auto":
		return true
	}
	return false
}

func lastTurnAgentIDUsed(conv *apiconv.Conversation) string {
	if conv == nil || len(conv.Transcript) == 0 {
		return ""
	}
	for i := len(conv.Transcript) - 1; i >= 0; i-- {
		t := conv.Transcript[i]
		if t == nil || t.AgentIdUsed == nil {
			continue
		}
		id := strings.TrimSpace(*t.AgentIdUsed)
		if id == "" || isAutoAgentRef(id) {
			continue
		}
		return id
	}
	return ""
}

func lastUserQueryText(conv *apiconv.Conversation) string {
	if conv == nil || len(conv.Transcript) == 0 {
		return ""
	}
	for ti := len(conv.Transcript) - 1; ti >= 0; ti-- {
		t := conv.Transcript[ti]
		if t == nil || len(t.Message) == 0 {
			continue
		}
		for mi := len(t.Message) - 1; mi >= 0; mi-- {
			m := t.Message[mi]
			if m == nil || !strings.EqualFold(strings.TrimSpace(m.Role), "user") || !strings.EqualFold(strings.TrimSpace(m.Type), "text") {
				continue
			}
			if m.Content == nil {
				continue
			}
			if s := strings.TrimSpace(*m.Content); s != "" {
				return s
			}
		}
	}
	return ""
}

func tokenizeText(text string) []string {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return nil
	}
	parts := strings.FieldsFunc(text, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsNumber(r))
	})
	if len(parts) == 0 {
		return nil
	}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func filterAutoSelectableAgents(agents []*agentmdl.Agent) []*agentmdl.Agent {
	if len(agents) == 0 {
		return nil
	}
	out := make([]*agentmdl.Agent, 0, len(agents))
	for _, a := range agents {
		if a == nil {
			continue
		}
		if a.Internal {
			continue
		}
		id := strings.TrimSpace(a.ID)
		if id == "" {
			id = strings.TrimSpace(a.Name)
		}
		if id == "" {
			continue
		}
		out = append(out, a)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func autoSelectAgentID(query string, candidates []*agentmdl.Agent) string {
	candidates = filterAutoSelectableAgents(candidates)
	if len(candidates) == 0 {
		return ""
	}
	queryTokens := tokenizeText(query)
	if len(queryTokens) == 0 {
		return ""
	}
	stopWords := map[string]struct{}{
		"a": {}, "an": {}, "and": {}, "are": {}, "as": {}, "at": {}, "be": {}, "but": {}, "by": {},
		"for": {}, "from": {}, "how": {}, "i": {}, "in": {}, "is": {}, "it": {}, "me": {}, "my": {},
		"of": {}, "on": {}, "or": {}, "please": {}, "the": {}, "to": {}, "we": {}, "with": {}, "you": {},
	}

	bestID := ""
	bestScore := 0
	for _, a := range candidates {
		if a == nil {
			continue
		}
		agentText := strings.Join([]string{
			strings.TrimSpace(a.ID),
			strings.TrimSpace(a.Name),
			strings.TrimSpace(a.Description),
		}, " ")
		agentTokens := tokenizeText(agentText)
		if len(agentTokens) == 0 {
			continue
		}
		tokenSet := map[string]struct{}{}
		for _, t := range agentTokens {
			tokenSet[t] = struct{}{}
		}
		score := 0
		for _, qt := range queryTokens {
			if _, skip := stopWords[qt]; skip {
				continue
			}
			if len(qt) < 3 {
				continue
			}
			if _, ok := tokenSet[qt]; ok {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			bestID = strings.TrimSpace(a.ID)
			if bestID == "" {
				bestID = strings.TrimSpace(a.Name)
			}
		}
	}
	if bestScore == 0 {
		return ""
	}
	return bestID
}

func (s *Service) resolveAgentIDForConversation(ctx context.Context, conv *apiconv.Conversation, query string) (string, bool, error) {
	providedQuery := strings.TrimSpace(query)
	if strings.TrimSpace(query) == "" {
		query = lastUserQueryText(conv)
	}

	defaultAgent := ""
	if s != nil && s.defaults != nil {
		defaultAgent = strings.TrimSpace(s.defaults.Agent)
	}

	explicitAgent := ""
	autoRequested := false
	if conv != nil && conv.AgentId != nil {
		explicitAgent = strings.TrimSpace(*conv.AgentId)
		if explicitAgent != "" && !isAutoAgentRef(explicitAgent) {
			return explicitAgent, false, nil
		}
		autoRequested = isAutoAgentRef(explicitAgent)
	} else {
		autoRequested = isAutoAgentRef(defaultAgent)
	}

	// When auto is not requested, preserve continuity by using the last agent that
	// executed in this conversation, before falling back to workspace defaults.
	if !autoRequested {
		if id := lastTurnAgentIDUsed(conv); id != "" {
			return id, false, nil
		}
		if defaultAgent != "" && !isAutoAgentRef(defaultAgent) {
			return defaultAgent, false, nil
		}
		return "", false, fmt.Errorf("agent is required")
	}

	var candidates []*agentmdl.Agent
	if s != nil && s.agentFinder != nil {
		if c, ok := s.agentFinder.(agentCatalog); ok {
			candidates = c.All()
		}
	}

	// Prefer LLM-based routing when available, then fall back to cheap token match.
	// Only run the LLM router when the caller provided a query for this turn.
	// This avoids extra LLM calls during internal operations such as summarization,
	// where the routing should rely on continuity (last used agent).
	if providedQuery != "" {
		if selected, err := s.classifyAgentIDWithLLM(ctx, conv, query, candidates); err != nil {
			return "", true, err
		} else if selected != "" {
			return selected, true, nil
		}
	}
	if selected := autoSelectAgentID(query, candidates); selected != "" {
		return selected, true, nil
	}

	// If routing cannot decide, keep continuity as a safe fallback.
	if id := lastTurnAgentIDUsed(conv); id != "" {
		return id, false, nil
	}
	if defaultAgent != "" && !isAutoAgentRef(defaultAgent) {
		return defaultAgent, false, nil
	}
	return "", true, fmt.Errorf("agent is required")
}
