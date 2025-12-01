package matcher

import (
	"github.com/viant/agently/genai/llm"
	"strings"
)

// Candidate represents a model scored along multiple dimensions.
type Candidate struct {
	ID           string
	Intelligence float64
	Speed        float64
	// Cost is a relative cost metric derived from pricing (higher means
	// more expensive). When zero, cost information is either unknown or
	// intentionally omitted.
	Cost float64
}

// Matcher holds a snapshot of all candidates and can pick the best one for
// given preferences.
type Matcher struct {
	cand []Candidate
}

// New builds a matcher from supplied candidates.
func New(c []Candidate) *Matcher { return &Matcher{cand: c} }

// Best returns ID of the highest-ranked candidate or "" when none.
func (m *Matcher) Best(p *llm.ModelPreferences) string {
	// 1. honour hints in order
	for _, h := range p.Hints {
		for _, c := range m.cand {
			if strings.Contains(c.ID, h) {
				return c.ID
			}
		}
	}

	// 2. compute normalized cost range when available
	minCost, maxCost := 0.0, 0.0
	firstCost := true
	for _, c := range m.cand {
		if c.Cost <= 0 {
			continue
		}
		if firstCost {
			minCost, maxCost = c.Cost, c.Cost
			firstCost = false
			continue
		}
		if c.Cost < minCost {
			minCost = c.Cost
		}
		if c.Cost > maxCost {
			maxCost = c.Cost
		}
	}
	useCost := !firstCost && maxCost > minCost && p.CostPriority > 0

	// 3. weight score (simple linear model with optional cost penalty)
	bestID := ""
	best := -1.0
	for _, c := range m.cand {
		s := p.IntelligencePriority*c.Intelligence + p.SpeedPriority*c.Speed
		if useCost && c.Cost > 0 {
			// Normalize cost into [0,1] and subtract a penalty so cheaper
			// models are preferred when CostPriority > 0.
			norm := (c.Cost - minCost) / (maxCost - minCost)
			s -= p.CostPriority * norm
		}
		if s > best {
			best, bestID = s, c.ID
		}
	}
	return bestID
}
