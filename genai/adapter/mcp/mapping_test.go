package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/genai/llm"
	mcpSchema "github.com/viant/mcp-protocol/schema"
)

func ptr(s string) *string {
	return &s
}

func TestMapTool(t *testing.T) {
	testCases := []struct {
		name     string
		tool     mcpSchema.Tool
		prefix   string
		expected llm.ToolDefinition
	}{
		{
			name: "no prefix",
			tool: mcpSchema.Tool{
				Name:        "testTool",
				Description: ptr("a test tool"),
				InputSchema: mcpSchema.ToolInputSchema{
					Properties: map[string]map[string]interface{}{
						"param1": {"type": "string"},
					},
					Required: []string{"param1"},
				},
			},
			prefix: "",
			expected: llm.ToolDefinition{
				Name:        "testTool",
				Description: "a test tool",
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": map[string]map[string]interface{}{"param1": {"type": "string"}},
				},
				Required: []string{"param1"},
			},
		},
		{
			name: "with prefix",
			tool: mcpSchema.Tool{
				Name:        "testTool",
				Description: ptr("a test tool"),
				InputSchema: mcpSchema.ToolInputSchema{
					Properties: map[string]map[string]interface{}{
						"param1": {"type": "string"},
					},
					Required: []string{"param1"},
				},
			},
			prefix: "pre_",
			expected: llm.ToolDefinition{
				Name:        "pre_testTool",
				Description: "a test tool",
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": map[string]map[string]interface{}{"param1": {"type": "string"}},
				},
				Required: []string{"param1"},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			def := MapTool(tc.tool, tc.prefix)
			assert.EqualValues(t, tc.expected, def)
		})
	}
}
