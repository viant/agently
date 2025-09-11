package adapter

import (
	"encoding/json"
	"strings"

	plan "github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/llm"
)

// ToolCallFromStep converts a StepOutcome to an llm.ToolCall.
func ToolCallFromStep(st *plan.StepOutcome) *llm.ToolCall {
	if st == nil || strings.TrimSpace(st.Name) == "" {
		return nil
	}
	var args map[string]interface{}
	if len(st.Request) > 0 {
		// best-effort parse; ignore errors to avoid breaking UX
		_ = json.Unmarshal(st.Request, &args)
	}
	call := &llm.ToolCall{
		ID:        strings.TrimSpace(st.ID),
		Name:      st.Name,
		Arguments: args,
		Result:    string(st.Response),
		Error:     st.Error,
	}
	return call
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
