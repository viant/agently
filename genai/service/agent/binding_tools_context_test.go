package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/viant/agently/genai/llm"
)

func TestApplyToolContext_DataDriven(t *testing.T) {
	type testCase struct {
		name            string
		initial         map[string]interface{}
		defs            []*llm.ToolDefinition
		expectToolsKey  string
		expectHasWebdrv bool
	}

	testCases := []testCase{
		{
			name:    "uses tools key when empty",
			initial: map[string]interface{}{},
			defs: []*llm.ToolDefinition{
				{Name: "webdriver-browserRun"},
				{Name: "resources.readImage"},
			},
			expectToolsKey:  "tools",
			expectHasWebdrv: true,
		},
		{
			name:    "uses agentlyTools when tools conflicts",
			initial: map[string]interface{}{"tools": "keep"},
			defs: []*llm.ToolDefinition{
				{Name: "resources-readImage"},
			},
			expectToolsKey:  "agentlyTools",
			expectHasWebdrv: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := cloneContextMap(tc.initial)
			applyToolContext(ctx, tc.defs)

			if tc.expectToolsKey == "agentlyTools" {
				assert.EqualValues(t, "keep", ctx["tools"])
			}

			raw := ctx[tc.expectToolsKey]
			toolsCtx, ok := raw.(map[string]interface{})
			require.EqualValues(t, true, ok)

			present, ok := toolsCtx["present"].(map[string]interface{})
			require.EqualValues(t, true, ok)
			services, ok := toolsCtx["services"].(map[string]interface{})
			require.EqualValues(t, true, ok)

			if tc.expectHasWebdrv {
				assert.EqualValues(t, true, toolsCtx["hasWebdriver"])
				assert.EqualValues(t, true, services["webdriver"])
				assert.EqualValues(t, true, present["webdriver-browserRun"])
			} else {
				assert.EqualValues(t, false, toolsCtx["hasWebdriver"])
			}
			assert.EqualValues(t, true, toolsCtx["hasResources"])
		})
	}
}
