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
	// No-op refinement: keep plan steps as-is.
	// We intentionally do not de-duplicate here to allow providers
	// (e.g. OpenAI /v1/responses) to receive all tool calls as issued.
	if p == nil {
		return
	}
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
