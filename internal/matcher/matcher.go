package matcher

import (
	"strconv"
	"strings"
	"time"

	"github.com/viant/agently/genai/llm"
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
	// BaseModel is the model name without provider prefix and version suffix.
	BaseModel string
	// Version carries a parsed version token (e.g., date or semver-ish) for tie-breaking.
	Version string
}

// Matcher holds a snapshot of all candidates and can pick the best one for
// given preferences.
type Matcher struct {
	cand []Candidate
}

// New builds a matcher from supplied candidates.
func New(c []Candidate) *Matcher {
	out := make([]Candidate, len(c))
	for i, cand := range c {
		if strings.TrimSpace(cand.BaseModel) == "" || strings.TrimSpace(cand.Version) == "" {
			base, ver := deriveBaseVersion(cand.ID)
			if cand.BaseModel == "" {
				cand.BaseModel = base
			}
			if cand.Version == "" {
				cand.Version = ver
			}
		}
		out[i] = cand
	}
	return &Matcher{cand: out}
}

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
	bestScore := -1.0
	bestIntel := -1.0
	var bestCand *Candidate
	for _, c := range cand {
		s := p.IntelligencePriority*c.Intelligence + p.SpeedPriority*c.Speed
		if useCost && c.Cost > 0 {
			// Normalize cost into [0,1] and subtract a penalty so cheaper
			// models are preferred when CostPriority > 0.
			norm := (c.Cost - minCost) / (maxCost - minCost)
			s -= p.CostPriority * norm
		}
		// Primary: prioritize intelligence, then score, then version.
		if c.Intelligence > bestIntel {
			bestIntel, bestScore, bestID, bestCand = c.Intelligence, s, c.ID, &c
			continue
		}
		if c.Intelligence < bestIntel {
			continue
		}
		if s > bestScore {
			bestScore, bestID, bestCand = s, c.ID, &c
			continue
		}
		if s < bestScore {
			continue
		}
		// Intelligence and score tie: prefer newer version of the same base model.
		if bestCand != nil && sameBase(bestCand, &c) {
			if newerVersion(&c, bestCand) {
				bestID, bestCand = c.ID, &c
			}
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

func sameBase(a, b *Candidate) bool {
	if a == nil || b == nil {
		return false
	}
	ab := strings.TrimSpace(strings.ToLower(a.BaseModel))
	bb := strings.TrimSpace(strings.ToLower(b.BaseModel))
	if ab == "" || bb == "" {
		return false
	}
	return ab == bb
}

func newerVersion(a, b *Candidate) bool {
	// Return true when candidate a has a newer version than b.
	if a == nil || b == nil {
		return false
	}
	av := strings.TrimSpace(a.Version)
	bv := strings.TrimSpace(b.Version)
	if av == "" || bv == "" {
		return false
	}
	if at, bt := parseDate(av), parseDate(bv); at != nil && bt != nil {
		return at.After(*bt)
	}
	if cmp, ok := compareNumericVersion(av, bv); ok {
		return cmp > 0
	}
	// Fallback: lexical compare.
	return av > bv
}

func parseDate(v string) *time.Time {
	// Support YYYY-MM-DD
	if len(v) != len("2006-01-02") {
		return nil
	}
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return nil
	}
	return &t
}

func compareNumericVersion(a, b string) (int, bool) {
	parse := func(s string) []int {
		s = strings.TrimSpace(strings.TrimPrefix(s, "v"))
		parts := strings.Split(s, ".")
		out := make([]int, 0, len(parts))
		for _, p := range parts {
			if p == "" {
				return nil
			}
			n, err := strconv.Atoi(p)
			if err != nil {
				return nil
			}
			out = append(out, n)
		}
		return out
	}
	as := parse(a)
	bs := parse(b)
	if as == nil || bs == nil {
		return 0, false
	}
	max := len(as)
	if len(bs) > max {
		max = len(bs)
	}
	for i := 0; i < max; i++ {
		ai, bi := 0, 0
		if i < len(as) {
			ai = as[i]
		}
		if i < len(bs) {
			bi = bs[i]
		}
		if ai > bi {
			return 1, true
		}
		if ai < bi {
			return -1, true
		}
	}
	return 0, true
}

func deriveBaseVersion(id string) (string, string) {
	src := strings.TrimSpace(id)
	if idx := strings.IndexByte(src, '_'); idx > 0 {
		src = strings.TrimSpace(src[idx+1:])
	}
	if src == "" {
		return "", ""
	}
	base := src
	ver := ""
	if i := strings.LastIndexByte(src, '-'); i > 0 && i+1 < len(src) {
		cand := strings.TrimSpace(src[i+1:])
		if isVersionToken(cand) {
			ver = cand
			base = strings.TrimSpace(src[:i])
		}
	}
	return base, ver
}

func isVersionToken(v string) bool {
	if v == "" {
		return false
	}
	if parseDate(v) != nil {
		return true
	}
	return isNumericVersion(v)
}

func isNumericVersion(v string) bool {
	v = strings.TrimSpace(strings.TrimPrefix(v, "v"))
	if v == "" {
		return false
	}
	parts := strings.Split(v, ".")
	for _, p := range parts {
		if p == "" {
			return false
		}
		if _, err := strconv.Atoi(p); err != nil {
			return false
		}
	}
	return true
}
