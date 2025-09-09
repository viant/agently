package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	plan "github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/llm"
)

func TestRefinePlan(t *testing.T) {
	// Helper to build a tool step quickly.
	mkStep := func(name string, args map[string]interface{}) plan.Step {
		return plan.Step{Type: "tool", Name: name, Args: args}
	}

	// Helper to build a prior result quickly.
	mkResult := func(name string, args map[string]interface{}) llm.ToolCall {
		return llm.ToolCall{Name: name, Args: args}
	}

	tests := []struct {
		name     string
		prior    []llm.ToolCall
		steps    plan.Steps
		expected plan.Steps
	}{
		{
			name:  "in-plan duplicate removed, keep first occurrence",
			prior: nil,
			steps: plan.Steps{
				mkStep("grep", map[string]interface{}{"q": "foo"}),
				mkStep("curl", map[string]interface{}{"url": "http://example.com"}),
				mkStep("grep", map[string]interface{}{"q": "foo"}), // duplicate
			},
			expected: plan.Steps{
				mkStep("grep", map[string]interface{}{"q": "foo"}),
				mkStep("curl", map[string]interface{}{"url": "http://example.com"}),
			},
		},
		{
			name: "prior results do not remove allowed repeat",
			prior: []llm.ToolCall{
				mkResult("calc", map[string]interface{}{"expr": "1+1"}),
			},
			steps: plan.Steps{
				mkStep("calc", map[string]interface{}{"expr": "1+1"}), // allowed repeat in later plan
				mkStep("notify", map[string]interface{}{"msg": "done"}),
			},
			expected: plan.Steps{
				mkStep("calc", map[string]interface{}{"expr": "1+1"}),
				mkStep("notify", map[string]interface{}{"msg": "done"}),
			},
		},
		{
			name:  "non-tool steps untouched",
			prior: nil,
			steps: plan.Steps{
				{Type: "elicitation", Content: "Need input"},
				{Type: "abort", Reason: "fatal"},
			},
			expected: plan.Steps{
				{Type: "elicitation", Content: "Need input"},
				{Type: "abort", Reason: "fatal"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &plan.Plan{Steps: append(plan.Steps(nil), tc.steps...)}
			RefinePlan(p, "")
			assert.EqualValues(t, tc.expected, p.Steps)
		})
	}
}
