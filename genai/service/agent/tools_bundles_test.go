package agent

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	agentmdl "github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/llm"
	toolbundle "github.com/viant/agently/genai/tool/bundle"
)

type fakeRegistry struct {
	defs []llm.ToolDefinition
}

func (r *fakeRegistry) Definitions() []llm.ToolDefinition { return r.defs }

func (r *fakeRegistry) MatchDefinition(pattern string) []*llm.ToolDefinition {
	pattern = strings.TrimSpace(pattern)
	var out []*llm.ToolDefinition
	for i := range r.defs {
		name := r.defs[i].Name
		if matchPattern(pattern, name) {
			d := r.defs[i]
			out = append(out, &d)
		}
	}
	return out
}

func matchPattern(pattern, name string) bool {
	if strings.TrimSpace(pattern) == "" {
		return false
	}
	pcanon := canon(pattern)
	ncanon := canon(name)
	if pcanon == "*" {
		return true
	}
	if pcanon == ncanon {
		return true
	}
	if strings.Contains(pcanon, "*") {
		prefix := strings.TrimSuffix(pcanon, "*")
		return strings.HasPrefix(ncanon, prefix)
	}
	// service-only pattern
	raw := strings.TrimSpace(pattern)
	if raw != "" && !strings.Contains(raw, ":") && !strings.Contains(raw, "*") {
		return strings.HasPrefix(ncanon, pcanon)
	}
	return false
}

func canon(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, ":", "_")
	return s
}

func (r *fakeRegistry) GetDefinition(string) (*llm.ToolDefinition, bool) { return nil, false }
func (r *fakeRegistry) MustHaveTools([]string) ([]llm.Tool, error)       { return nil, nil }
func (r *fakeRegistry) Execute(context.Context, string, map[string]interface{}) (string, error) {
	return "", fmt.Errorf("not implemented")
}
func (r *fakeRegistry) SetDebugLogger(io.Writer)                 {}
func (r *fakeRegistry) Initialize(context.Context)               {}
func (r *fakeRegistry) ToolTimeout(string) (time.Duration, bool) { return 0, false }

func TestResolveTools_WithBundles(t *testing.T) {
	testCases := []struct {
		name        string
		query       *QueryInput
		bundles     []*toolbundle.Bundle
		defs        []llm.ToolDefinition
		expectNames []string
	}{
		{
			name: "runtime_bundle_selection_expands_tools",
			query: &QueryInput{
				ToolBundles: []string{"resources"},
				Agent:       &agentmdl.Agent{Tool: agentmdl.Tool{Bundles: []string{"system"}}},
			},
			bundles: []*toolbundle.Bundle{
				{
					ID: "resources",
					Match: []toolbundle.MatchRule{
						{Name: "resources/*", Exclude: []string{"resources:matchDocuments"}},
					},
				},
			},
			defs: []llm.ToolDefinition{
				{Name: "resources:read"},
				{Name: "resources:list"},
				{Name: "resources:matchDocuments"},
				{Name: "system/exec:execute"},
			},
			expectNames: []string{"resources:list", "resources:read"},
		},
		{
			name: "agent_bundle_selection_used_when_no_runtime_override",
			query: &QueryInput{
				Agent: &agentmdl.Agent{Tool: agentmdl.Tool{Bundles: []string{"system"}}},
			},
			bundles: []*toolbundle.Bundle{
				{
					ID: "system",
					Match: []toolbundle.MatchRule{
						{Name: "system/*"},
					},
				},
			},
			defs: []llm.ToolDefinition{
				{Name: "resources:read"},
				{Name: "system/exec:execute"},
				{Name: "system/os:getEnv"},
			},
			expectNames: []string{"system/exec:execute", "system/os:getEnv"},
		},
		{
			name: "empty_tools_allowed_does_not_disable_bundle_resolution",
			query: &QueryInput{
				ToolsAllowed: []string{},
				Agent:        &agentmdl.Agent{Tool: agentmdl.Tool{Bundles: []string{"system"}}},
			},
			bundles: []*toolbundle.Bundle{
				{
					ID: "system",
					Match: []toolbundle.MatchRule{
						{Name: "system/*"},
					},
				},
			},
			defs: []llm.ToolDefinition{
				{Name: "system/exec:execute"},
				{Name: "system/os:getEnv"},
			},
			expectNames: []string{"system/exec:execute", "system/os:getEnv"},
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
			ctx, _ := withWarnings(context.Background())
			actual, err := svc.resolveTools(ctx, tc.query)
			assert.EqualValues(t, nil, err)
			var got []string
			for _, t := range actual {
				got = append(got, t.Definition.Name)
			}
			assert.EqualValues(t, tc.expectNames, got)
		})
	}
}
