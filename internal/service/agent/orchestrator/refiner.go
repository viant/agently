package orchestrator

import (
	"github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/llm"
)

// ToolKey uniquely identifies a tool call by its name and canonicalised
// arguments JSON.  Hashable so it can be used as a map key.
type ToolKey struct {
	Name string
	Args string
}

func RefinePlan(p *plan.Plan) {
	if len(p.Steps) == 0 {
		return
	}

	// ------------------------------------------------------------------
	// Two-tier duplicate protection:
	//   1. Inside a single Plan we *remove* redundant repetitions because
	//      running the exact same tool call twice in the same batch is never
	//      useful.
	// 2. Across iterations, we no longer discard the step here.
	//    The executor is now responsible for detecting if an identical call
	//    has already been executed, and if so, it will short-circuit the execution
	//    and return the previous result.
	// ------------------------------------------------------------------

	seenThisPlan := map[ToolKey]struct{}{}
	filtered := make(plan.Steps, 0, len(p.Steps))

	for _, st := range p.Steps {
		if st.Type != "tool" {
			filtered = append(filtered, st)
			continue
		}

		// Remove duplicates that occur *within* the same Plan only.
		tk := ToolKey{st.Name, CanonicalArgs(st.Args)}
		if _, ok := seenThisPlan[tk]; ok {
			continue
		}

		seenThisPlan[tk] = struct{}{}
		filtered = append(filtered, st)
	}

	p.Steps = filtered
}

// dedupResultsSkipSeenErrors returns a new slice containing only the last occurrence
// of each (Name, canonical Args) pair, preserving their chronological order.
// It skips results that have an Error and were already seen by the LLM during the latest plan generation.
func dedupResultsSkipSeenErrors(in []llm.ToolCall) []*llm.ToolCall {
	outRev := make([]*llm.ToolCall, 0, len(in))
	if len(in) <= 1 {
		for i := range in {
			outRev = append(outRev, &in[i])
		}
		return outRev
	}

	type key struct {
		Name string
		Args string
	}

	seen := make(map[key]struct{}, len(in))

	// Walk backwards so we keep the *last* occurrence.
	for i := len(in) - 1; i >= 0; i-- {

		k := key{in[i].Name, CanonicalArgs(in[i].Arguments)}
		if _, dup := seen[k]; dup {
			continue
		}

		// Note: Seen flag is no longer tracked on llm.ToolCall; include errors as well.

		seen[k] = struct{}{}
		outRev = append(outRev, &in[i])
	}

	// Reverse back to chronological order.
	for i, j := 0, len(outRev)-1; i < j; i, j = i+1, j-1 {
		outRev[i], outRev[j] = outRev[j], outRev[i]
	}
	return outRev
}
