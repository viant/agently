package expose

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/genai/llm"
	mcpschema "github.com/viant/mcp-protocol/schema"
)

func TestResolveToolName(t *testing.T) {
	testCases := []struct {
		description string
		raw         string
		defs        []llm.ToolDefinition
		expected    string
		found       bool
		hasError    bool
	}{
		{
			description: "full name exact match",
			raw:         "system/exec:execute",
			defs: []llm.ToolDefinition{
				{Name: "system/exec:execute"},
			},
			expected: "system/exec:execute",
			found:    true,
		},
		{
			description: "method only resolves unique",
			raw:         "execute",
			defs: []llm.ToolDefinition{
				{Name: "system/exec:execute"},
				{Name: "system/patch:apply"},
			},
			expected: "system/exec:execute",
			found:    true,
		},
		{
			description: "method only not found",
			raw:         "missing",
			defs: []llm.ToolDefinition{
				{Name: "system/exec:execute"},
			},
			found: false,
		},
		{
			description: "method only ambiguous",
			raw:         "execute",
			defs: []llm.ToolDefinition{
				{Name: "system/exec:execute"},
				{Name: "other/exec:execute"},
			},
			hasError: true,
		},
	}

	for _, tc := range testCases {
		actual, found, err := resolveToolName(tc.raw, tc.defs)
		if tc.hasError {
			assert.NotNil(t, err, tc.description)
			continue
		}
		assert.Nil(t, err, tc.description)
		assert.EqualValues(t, tc.found, found, tc.description)
		assert.EqualValues(t, tc.expected, actual, tc.description)
	}
}

func TestToolAllowed(t *testing.T) {
	testCases := []struct {
		description string
		patterns    []string
		name        string
		expected    bool
	}{
		{
			description: "service only matches any method",
			patterns:    []string{"system/exec"},
			name:        "system/exec:execute",
			expected:    true,
		},
		{
			description: "service only does not match other service",
			patterns:    []string{"system/exec"},
			name:        "system/patch:apply",
			expected:    false,
		},
		{
			description: "short service matches",
			patterns:    []string{"orchestration"},
			name:        "orchestration:updatePlan",
			expected:    true,
		},
		{
			description: "wildcard matches prefix",
			patterns:    []string{"system/*"},
			name:        "system/exec:execute",
			expected:    true,
		},
	}

	for _, tc := range testCases {
		actual := toolAllowed(tc.patterns, tc.name)
		assert.EqualValues(t, tc.expected, actual, tc.description)
	}
}

func TestMcpToolFromDefinition(t *testing.T) {
	testCases := []struct {
		description string
		def         *llm.ToolDefinition
		expected    *mcpschema.Tool
	}{
		{
			description: "basic conversion includes required and properties",
			def: &llm.ToolDefinition{
				Name:        "system/exec:execute",
				Description: "exec",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"command": map[string]interface{}{"type": "string"},
					},
				},
				Required: []string{"command"},
				OutputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"status": map[string]interface{}{"type": "integer"},
					},
				},
			},
			expected: &mcpschema.Tool{
				Name:        "system/exec:execute",
				Description: ptr("exec"),
				InputSchema: mcpschema.ToolInputSchema{
					Type:       "object",
					Properties: mcpschema.ToolInputSchemaProperties(map[string]map[string]interface{}{"command": {"type": "string"}}),
					Required:   []string{"command"},
				},
				OutputSchema: &mcpschema.ToolOutputSchema{
					Type:       "object",
					Properties: map[string]map[string]interface{}{"status": {"type": "integer"}},
				},
			},
		},
	}

	for _, tc := range testCases {
		actual := mcpToolFromDefinition(tc.def)
		assert.EqualValues(t, tc.expected, actual, tc.description)
	}
}

func ptr(s string) *string { return &s }
