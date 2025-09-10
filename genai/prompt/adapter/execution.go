package adapter

import (
	"strings"

	plan "github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/llm"
)

// ToolCallFromStep converts a StepOutcome to an llm.ToolCall.
func ToolCallFromStep(st *plan.StepOutcome) *llm.ToolCall {
	if st == nil || strings.TrimSpace(st.Name) == "" {
		return nil
	}
	summary := strings.TrimSpace(st.Reason)
	if summary == "" && len(st.Response) > 0 {
		summary = trimStr(string(st.Response), 160)
	}
	return &llm.ToolCall{Name: st.Name, Result: summary, Error: st.Error}
}

// ToolCallsFromOutcomes flattens outcomes into llm.ToolCall slice.
func ToolCallsFromOutcomes(out []*plan.Outcome) []*llm.ToolCall {
	var res []*llm.ToolCall
	for _, oc := range out {
		if oc == nil {
			continue
		}
		for _, st := range oc.Steps {
			if call := ToolCallFromStep(st); call != nil {
				res = append(res, call)
			}
		}
	}
	return res
}

func trimStr(s string, n int) string {
	s = strings.TrimSpace(s)
	if n <= 0 || len(s) <= n {
		return s
	}
	if n > 3 {
		return s[:n-3] + "..."
	}
	return s[:n]
}
