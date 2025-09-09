package adapter

import (
	"strings"

	plan "github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/prompt"
)

// ToolCallFromStep converts a StepOutcome to a prompt.ToolCall.
func ToolCallFromStep(st *plan.StepOutcome) *prompt.ToolCall {
	if st == nil || strings.TrimSpace(st.Name) == "" {
		return nil
	}
	status := "failed"
	if st.Success {
		status = "completed"
	}
	summary := strings.TrimSpace(st.Reason)
	if summary == "" && len(st.Response) > 0 {
		summary = trimStr(string(st.Response), 160)
	}
	return &prompt.ToolCall{Name: st.Name, Status: status, Result: summary, Error: st.Error, Elapsed: st.Elapsed}
}

// ToolCallsFromOutcomes flattens outcomes into prompt.ToolCall slice.
func ToolCallsFromOutcomes(out []*plan.Outcome) []*prompt.ToolCall {
	var res []*prompt.ToolCall
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
