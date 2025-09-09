package core

import (
	plan "github.com/viant/agently/genai/llm"
)

// toolKey uniquely identifies a tool invocation by its name and canonicalised arguments.
type toolKey struct {
	Name string
	Args string
}

// DuplicateGuard tracks the recent sequence of calls, detects pathological repetition patterns, and applies a set
// of heuristics to decide whether a newly–proposed call should be blocked.
type DuplicateGuard struct {
	lastKey     toolKey
	consecutive int
	window      []toolKey
	latest      map[toolKey]plan.ToolCall // most recent result for each key
}

const (
	consecutiveLimit = 3 // block on the 3rd identical consecutive call
	windowSize       = 8 // sliding window length
	windowFreqLimit  = 5 // block if ≥5 occurrences inside the window
)

// NewDuplicateGuard returns a guard pre‑seeded with prior results.
func NewDuplicateGuard(prior []plan.ToolCall) *DuplicateGuard {
	g := &DuplicateGuard{
		latest: make(map[toolKey]plan.ToolCall, len(prior)),
		window: make([]toolKey, 0, windowSize),
	}
	for _, r := range prior {
		key := g.key(r.Name, r.Args)
		g.latest[key] = r
		g.window = append(g.window, key)
		g.lastKey = key
	}
	return g
}

func (g *DuplicateGuard) key(name string, args map[string]interface{}) toolKey {
	return toolKey{Name: name, Args: CanonicalArgs(args)}
}

// ShouldBlock reports whether the proposed call should be blocked and, if so,
// returns the latest cached result for the same key (if any).
//
// Blocking heuristics (evaluated in order):
//  1. Immediate repeat of a *successful* call.
//  2. The same call executed `consecutiveLimit` times in a row.
//  3. The call appears at least `windowFreqLimit` times inside the sliding
//     window of size `windowSize`.
//  4. The window contains only two distinct calls that alternate (A, B,
//     A, B, …).
func (g *DuplicateGuard) ShouldBlock(name string, args map[string]interface{}) (bool, plan.ToolCall) {
	key := g.key(name, args)
	prev := g.latest[key] // previous result for the same key, if any

	// Heuristic #1: immediately repeated *successful* call.
	if key == g.lastKey && prev.Name != "" && prev.Error == "" {
		return true, prev
	}

	// Update counters and window state.
	g.updateConsecutive(key)
	g.updateWindow(key)

	// Apply remaining heuristics.
	if g.consecutive >= consecutiveLimit {
		return true, prev
	}
	if g.frequency(key) >= windowFreqLimit {
		return true, prev
	}

	if g.isAlternatingPattern() {
		return true, prev
	}

	return false, plan.ToolCall{}
}

// updateConsecutive increments or resets the consecutive‑repeat counter.
func (g *DuplicateGuard) updateConsecutive(k toolKey) {
	if k == g.lastKey {
		g.consecutive++
	} else {
		g.consecutive = 1
		g.lastKey = k
	}
}

// updateWindow appends k to the sliding window, trimming it to windowSize.
func (g *DuplicateGuard) updateWindow(k toolKey) {
	g.window = append(g.window, k)
	if len(g.window) > windowSize {
		g.window = g.window[len(g.window)-windowSize:]
	}
}

// frequency counts occurrences of k inside the current window.
func (g *DuplicateGuard) frequency(k toolKey) int {
	count := 0
	for _, w := range g.window {
		if w == k {
			count++
		}
	}
	return count
}

// isAlternatingPattern reports true when the window consists of exactly two
// distinct keys that alternate without deviation (A, B, A, B, …).
func (g *DuplicateGuard) isAlternatingPattern() bool {
	if len(g.window) < windowSize {
		return false
	}

	alternating := false
	distinct := map[toolKey]struct{}{}
	for _, w := range g.window {
		distinct[w] = struct{}{}
	}

	if len(distinct) == 2 {
		alternating = true
		for i := 2; i < len(g.window); i++ {
			if g.window[i] != g.window[i-2] {
				alternating = false
				break
			}
		}
	}

	return alternating
}

// RegisterResult stores latest outcome for reuse.
func (g *DuplicateGuard) RegisterResult(name string, args map[string]interface{}, res plan.ToolCall) {
	k := g.key(name, args)
	g.latest[k] = res
	g.lastKey = k
}
