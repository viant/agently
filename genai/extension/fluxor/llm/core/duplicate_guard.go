package core

import (
	plan "github.com/viant/agently/genai/agent/plan"
)

// toolKey uniquely identifies a tool invocation by its name and canonicalised arguments.
type toolKey struct {
	Name string
	Args string
}

// DuplicateGuard detects pathological repetition patterns of tool calls across
// iterations and inside a single plan.
type DuplicateGuard struct {
	lastKey     toolKey
	consecutive int
	window      []toolKey
	latest      map[toolKey]plan.Result // most recent result for each key
}

const (
	consecutiveLimit = 3 // 3rd identical consecutive call will be blocked
	windowSize       = 8 // sliding window length
	windowFreqLimit  = 4 // >=4 occurrences inside window triggers block
)

// NewDuplicateGuard initialises a guard primed with any prior results.
func NewDuplicateGuard(prior []plan.Result) *DuplicateGuard {
	g := &DuplicateGuard{latest: make(map[toolKey]plan.Result, len(prior))}
	for _, r := range prior {
		g.latest[g.key(r.Name, r.Args)] = r
	}
	return g
}

func (g *DuplicateGuard) key(name string, args map[string]interface{}) toolKey {
	return toolKey{Name: name, Args: CanonicalArgs(args)}
}

// ShouldBlock updates internal state with the proposed call and returns
// whether it should be blocked together with the previous result if present.
func (g *DuplicateGuard) ShouldBlock(name string, args map[string]interface{}) (bool, plan.Result) {
	k := g.key(name, args)

	// ------------------------------------------------------------------
	// Check whether we have executed this exact tool call (same name and
	// canonicalised args) in one of the *prior* iterations.  While repeating
	// such a call may sometimes be wasteful, forbidding it altogether turned
	// out to be too restrictive in practice – certain idempotent or
	// lightweight tools (for example simple arithmetic via "calc") are
	// perfectly fine to rerun when the planner explicitly requests it.
	//
	// Therefore we no longer block immediately.  Instead, the duplicate will
	// still be detected by the sliding-window & consecutive heuristics below
	// which focus on pathological repetition patterns (loops) rather than a
	// single re-execution.
	// ------------------------------------------------------------------

	prev, _ := g.latest[k] // keep for potential return later

	// Certain tools are safe and inexpensive to execute multiple times.  Allow
	// them to bypass the "prior successful result" optimisation so planners
	// can intentionally re-run them across iterations.
	var repeatAllowed = map[string]struct{}{
		"calc": {}, // pure function – deterministic & idempotent
	}

	if prev.Name != "" && prev.Error == "" {
		if _, ok := repeatAllowed[name]; ok {
			// Explicitly permitted to repeat – fall through to regular loop
			// detection heuristics instead of blocking immediately.
		} else {
			return true, prev
		}
	}

	if k == g.lastKey {
		g.consecutive++
	} else {
		g.consecutive = 1
		g.lastKey = k
	}

	// sliding window update
	g.window = append(g.window, k)
	if len(g.window) > windowSize {
		g.window = g.window[1:]
	}

	block := false
	if g.consecutive >= consecutiveLimit {
		block = true
	} else {
		freq := 0
		for _, w := range g.window {
			if w == k {
				freq++
			}
		}
		if freq >= windowFreqLimit {
			block = true
		} else if len(g.window) == windowSize {
			distinct := map[toolKey]struct{}{}
			for _, w := range g.window {
				distinct[w] = struct{}{}
			}
			if len(distinct) == 2 {
				alternating := true
				for i := 2; i < len(g.window); i++ {
					if g.window[i] != g.window[i-2] {
						alternating = false
						break
					}
				}
				if alternating {
					block = true
				}
			}
		}
	}

	if !block {
		return false, plan.Result{}
	}
	return true, prev
}

// RegisterResult stores latest outcome for reuse.
func (g *DuplicateGuard) RegisterResult(name string, args map[string]interface{}, res plan.Result) {
	g.latest[g.key(name, args)] = res
}
