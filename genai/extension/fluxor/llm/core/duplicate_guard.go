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
	// Fast-path: If we have already executed this exact tool call (same
	// name and canonicalised args) in a *prior* iteration **and** it
	// succeeded, short-circuit immediately. Invoking the tool again is
	// wasteful and – for idempotent tools – yields an identical result,
	// confusing the planner and potentially causing loops.
	//
	// Re-execution is still allowed when the previous attempt ended in an
	// error so the plan has a chance to recover once the missing
	// parameter/condition is fixed.
	// ------------------------------------------------------------------
	if prev, ok := g.latest[k]; ok {
		if prev.Error == "" { // previous run succeeded – block duplicate
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
	prev, _ := g.latest[k]
	return true, prev
}

// RegisterResult stores latest outcome for reuse.
func (g *DuplicateGuard) RegisterResult(name string, args map[string]interface{}, res plan.Result) {
	g.latest[g.key(name, args)] = res
}
