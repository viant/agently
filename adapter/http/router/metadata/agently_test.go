package metadata

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/genai/agent"
	execsvc "github.com/viant/agently/genai/executor"
	execcfg "github.com/viant/agently/genai/executor/config"
	"github.com/viant/agently/genai/llm"
	modelprovider "github.com/viant/agently/genai/llm/provider"
	mcpcfg "github.com/viant/fluxor-mcp/mcp/config"
)

func TestAggregate(t *testing.T) {
	testCases := []struct {
		name    string
		cfg     *execsvc.Config
		defs    []llm.ToolDefinition
		want    *AgentlyResponse
		wantErr bool
	}{
		{
			name: "basic with names",
			cfg: &execsvc.Config{
				Default: execcfg.Defaults{Agent: "chat", Model: "gpt-4o"},
				Agent: &mcpcfg.Group[*agent.Agent]{
					Items: []*agent.Agent{{Identity: agent.Identity{Name: "Chat"}}},
				},
				Model: &mcpcfg.Group[*modelprovider.Config]{
					Items: []*modelprovider.Config{{ID: "gpt-4o"}},
				},
			},
			defs: []llm.ToolDefinition{{Name: "fs_read"}},
			want: &AgentlyResponse{
				Agents: []string{"Chat"},
				Tools:  []string{"fs_read"},
				Models: []string{"gpt-4o"},
			},
		},
		{
			name: "agent tool matching",
			cfg: &execsvc.Config{
				Default: execcfg.Defaults{Agent: "Chat", Model: "gpt"},
				Agent: &mcpcfg.Group[*agent.Agent]{
					Items: []*agent.Agent{{
						Identity: agent.Identity{Name: "Chat"},
						Tool:     []*llm.Tool{{Pattern: "workflow-"}},
					}},
				},
			},
			defs: []llm.ToolDefinition{{Name: "workflow-run"}, {Name: "system_patch-apply"}},
			want: &AgentlyResponse{
				Agents:     []string{"Chat"},
				Tools:      []string{"system_patch-apply", "workflow-run"},
				AgentTools: map[string][]string{"Chat": {"workflow-run"}},
			},
		},
		{
			name: "fallback to ID and embedder default",
			cfg: &execsvc.Config{
				Default: execcfg.Defaults{Agent: "agentA", Model: "m1", Embedder: "e1"},
				Agent: &mcpcfg.Group[*agent.Agent]{
					Items: []*agent.Agent{{Identity: agent.Identity{ID: "agentA"}}},
				},
				Model: &mcpcfg.Group[*modelprovider.Config]{
					Items: []*modelprovider.Config{{ID: "m1"}, {ID: "m2"}},
				},
			},
			defs: []llm.ToolDefinition{{Name: "b"}, {Name: "a"}},
			want: &AgentlyResponse{
				Agents: []string{"agentA"},
				Tools:  []string{"a", "b"},
				Models: []string{"m1", "m2"},
			},
		},
		{
			name:    "nil config error",
			cfg:     nil,
			defs:    nil,
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		got, err := Aggregate(tc.cfg, tc.defs)
		if tc.wantErr {
			assert.Error(t, err, tc.name)
			continue
		}
		assert.NoError(t, err, tc.name)
		// Fill defaults in expected struct to match Aggregate behaviour
		if tc.want != nil && tc.cfg != nil {
			tc.want.Defaults.Agent = tc.cfg.Default.Agent
			tc.want.Defaults.Model = tc.cfg.Default.Model
			tc.want.Defaults.Embedder = tc.cfg.Default.Embedder
		}
		assert.EqualValues(t, tc.want, got, tc.name)
	}
}
