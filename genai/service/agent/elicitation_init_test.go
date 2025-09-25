package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/genai/agent/plan"
	mcpproto "github.com/viant/mcp-protocol/schema"
)

func Test_missingRequired(t *testing.T) {
	tests := []struct {
		name     string
		schema   *plan.Elicitation
		ctx      map[string]any
		expected []string
	}{
		{
			name: "all present",
			schema: &plan.Elicitation{ElicitRequestParams: mcpproto.ElicitRequestParams{RequestedSchema: mcpproto.ElicitRequestParamsRequestedSchema{
				Required: []string{"projectId", "region"},
			}}},
			ctx:      map[string]any{"projectId": "p1", "region": "us"},
			expected: []string{},
		},
		{
			name: "one missing",
			schema: &plan.Elicitation{ElicitRequestParams: mcpproto.ElicitRequestParams{RequestedSchema: mcpproto.ElicitRequestParamsRequestedSchema{
				Required: []string{"projectId", "region"},
			}}},
			ctx:      map[string]any{"projectId": "p1"},
			expected: []string{"region"},
		},
		{
			name: "nil ctx => all required",
			schema: &plan.Elicitation{ElicitRequestParams: mcpproto.ElicitRequestParams{RequestedSchema: mcpproto.ElicitRequestParamsRequestedSchema{
				Required: []string{"a", "b"},
			}}},
			ctx:      nil,
			expected: []string{"a", "b"},
		},
		{
			name: "whitespace string treated missing",
			schema: &plan.Elicitation{ElicitRequestParams: mcpproto.ElicitRequestParams{RequestedSchema: mcpproto.ElicitRequestParamsRequestedSchema{
				Required: []string{"token"},
			}}},
			ctx:      map[string]any{"token": "   "},
			expected: []string{"token"},
		},
		{
			name:     "no required",
			schema:   &plan.Elicitation{ElicitRequestParams: mcpproto.ElicitRequestParams{RequestedSchema: mcpproto.ElicitRequestParamsRequestedSchema{}}},
			ctx:      map[string]any{"x": 1},
			expected: []string{},
		},
	}

	for _, tc := range tests {
		got := missingRequired(tc.schema, tc.ctx)
		assert.EqualValues(t, tc.expected, got)
	}
}
