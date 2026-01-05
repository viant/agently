package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	agentmdl "github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/llm"
	toolbundle "github.com/viant/agently/genai/tool/bundle"
)

func TestService_BuildToolSignatures_WithBundles(t *testing.T) {
	testCases := []struct {
		name        string
		input       *QueryInput
		bundles     []*toolbundle.Bundle
		defs        []llm.ToolDefinition
		expectNames []string
	}{
		{
			name: "bundles_only_includes_signatures",
			input: &QueryInput{
				Agent: &agentmdl.Agent{
					Tool: agentmdl.Tool{Bundles: []string{"system"}},
				},
			},
			bundles: []*toolbundle.Bundle{
				{ID: "system", Match: []toolbundle.MatchRule{{Name: "system/*"}}},
			},
			defs: []llm.ToolDefinition{
				{Name: "system/exec:execute"},
				{Name: "system/os:getEnv"},
				{Name: "resources:read"},
			},
			expectNames: []string{"system_exec-execute", "system_os-getEnv"},
		},
		{
			name: "no_tool_config_returns_empty",
			input: &QueryInput{
				Agent: &agentmdl.Agent{},
			},
			bundles:     nil,
			defs:        []llm.ToolDefinition{{Name: "system/exec:execute"}},
			expectNames: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reg := &fakeRegistry{defs: tc.defs}
			svc := &Service{
				registry: reg,
				toolBundles: func(ctx context.Context) ([]*toolbundle.Bundle, error) {
					return tc.bundles, nil
				},
			}
			actual, _, err := svc.buildToolSignatures(context.Background(), tc.input)
			assert.EqualValues(t, nil, err)
			var got []string
			for _, d := range actual {
				got = append(got, d.Name)
			}
			assert.EqualValues(t, tc.expectNames, got)
		})
	}
}
