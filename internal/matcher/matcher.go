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
	// 1) Optional provider reduction: collect provider hints and filter
	// the candidate set when present. Provider is the prefix before the first '_'.
	providermap := map[string]struct{}{}
	for _, h := range p.Hints {
		hv := strings.ToLower(strings.TrimSpace(h))
		if hv == "" {
			continue
		}
		// Check if hv matches any provider prefix among candidates
		for _, c := range m.cand {
			id := strings.ToLower(strings.TrimSpace(c.ID))
			if idx := strings.IndexByte(id, '_'); idx > 0 {
				if id[:idx] == hv {
					providermap[hv] = struct{}{}
				}
			}
		}
	}
	cand := m.cand
	if len(providermap) > 0 {
		filtered := make([]Candidate, 0, len(m.cand))
		for _, c := range m.cand {
			id := strings.ToLower(strings.TrimSpace(c.ID))
			if idx := strings.IndexByte(id, '_'); idx > 0 {
				if _, ok := providermap[id[:idx]]; ok {
					filtered = append(filtered, c)
				}
			}
		}
		if len(filtered) > 0 {
			cand = filtered
		}
	}

	// 2) Honour model hints (token-aware) in order within the reduced set
	for _, h := range p.Hints {
		hv := strings.ToLower(strings.TrimSpace(h))
		if hv == "" {
			continue
		}
		// Skip provider-only hints here; they already reduced the set
		isProvider := false
		for _, c := range cand {
			id := strings.ToLower(strings.TrimSpace(c.ID))
			if idx := strings.IndexByte(id, '_'); idx > 0 && id[:idx] == hv {
				isProvider = true
				break
			}
		}
		if isProvider {
			continue
		}
		for _, c := range cand {
			if hintMatches(c.ID, hv) {
				return c.ID
			}
		}
	}

	// 2. compute normalized cost range when available
	minCost, maxCost := 0.0, 0.0
	firstCost := true
	for _, c := range cand {
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
	for _, c := range cand {
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

// hintMatches returns true when hint matches candidate id using
// provider-prefix or token-aware model matching to avoid accidental
// substrings (e.g., "mini" should not match "gemini").
func hintMatches(candidateID, hint string) bool {
	id := strings.ToLower(strings.TrimSpace(candidateID))
	h := strings.ToLower(strings.TrimSpace(hint))
	if id == "" || h == "" {
		return false
	}
	// Exact id match
	if id == h {
		return true
	}
	// Provider exact match: prefix before first underscore equals hint
	if idx := strings.IndexByte(id, '_'); idx > 0 {
		prov := id[:idx]
		model := id[idx+1:]
		if prov == h {
			return true
		}
		// Token-aware model match: ensure boundaries around hint
		if containsToken(model, h) {
			return true
		}
		return false
	}
	// Fallback: token-aware match on entire id
	return containsToken(id, h)
}

func containsToken(s, tok string) bool {
	if s == "" || tok == "" {
		return false
	}
	i := 0
	for {
		j := strings.Index(s[i:], tok)
		if j < 0 {
			return false
		}
		start := i + j
		end := start + len(tok)
		beforeOK := start == 0 || isSep(s[start-1])
		afterOK := end == len(s) || isSep(s[end]) || isDigit(s[end])
		if beforeOK && afterOK {
			return true
		}
		i = end
	}
}

func isSep(b byte) bool {
	switch b {
	case '-', '_', '.', ':', '/':
		return true
	default:
		return false
	}
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }
