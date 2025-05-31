package matcher

import "strings"

// Candidate represents a model scored along multiple dimensions.
type Candidate struct {
    ID           string
    Intelligence float64
    Speed        float64
}

// Preferences expresses caller priorities (0..1) + optional name hints.
type Preferences struct {
    Intelligence float64
    Speed        float64
    Hints        []string
}

// Matcher holds a snapshot of all candidates and can pick the best one for
// given preferences.
type Matcher struct {
    cand []Candidate
}

// New builds a matcher from supplied candidates.
func New(c []Candidate) *Matcher { return &Matcher{cand: c} }

// Best returns ID of the highest-ranked candidate or "" when none.
func (m *Matcher) Best(p Preferences) string {
    // 1. honour hints in order
    for _, h := range p.Hints {
        for _, c := range m.cand {
            if strings.Contains(c.ID, h) {
                return c.ID
            }
        }
    }

    // 2. weight score (simple linear model)
    bestID := ""
    best := -1.0
    for _, c := range m.cand {
        s := p.Intelligence*c.Intelligence + p.Speed*c.Speed
        if s > best {
            best, bestID = s, c.ID
        }
    }
    return bestID
}
