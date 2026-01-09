package chat

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/viant/agently/genai/agent"
)

type agentCatalog interface {
	All() []*agent.Agent
}

func isAutoAgentRef(agentRef string) bool {
	switch strings.ToLower(strings.TrimSpace(agentRef)) {
	case "agent_id", "auto":
		return true
	}
	return false
}

func normalizeConversationAgent(agentRef string) string {
	if isAutoAgentRef(agentRef) {
		return ""
	}
	return strings.TrimSpace(agentRef)
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

func filterAutoSelectableAgents(agents []*agent.Agent) []*agent.Agent {
	if len(agents) == 0 {
		return nil
	}
	out := make([]*agent.Agent, 0, len(agents))
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

func autoSelectAgentID(query string, candidates []*agent.Agent) string {
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

func resolveAgentID(reqAgent, conversationAgent, defaultAgent, query string, candidates []*agent.Agent) (string, bool, error) {
	reqAgent = strings.TrimSpace(reqAgent)
	conversationAgent = normalizeConversationAgent(conversationAgent)
	defaultAgent = strings.TrimSpace(defaultAgent)

	if reqAgent != "" && !isAutoAgentRef(reqAgent) {
		return reqAgent, false, nil
	}
	// Preserve legacy behavior: empty request agent uses conversation agent when set.
	if reqAgent == "" && conversationAgent != "" {
		return conversationAgent, false, nil
	}

	if selected := autoSelectAgentID(query, candidates); selected != "" {
		return selected, true, nil
	}
	if conversationAgent != "" {
		return conversationAgent, false, nil
	}
	if defaultAgent != "" {
		return defaultAgent, false, nil
	}
	return "", false, fmt.Errorf("agent is required")
}
