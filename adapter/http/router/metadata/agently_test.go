package metadata

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/genai/agent"
	execsvc "github.com/viant/agently/genai/executor"
	execcfg "github.com/viant/agently/genai/executor/config"
	"github.com/viant/agently/genai/llm"
	modelprovider "github.com/viant/agently/genai/llm/provider"
	mcpcfg "github.com/viant/agently/internal/mcp/config"
)

func TestAggregate(t *testing.T) {
	testCases := []struct {
		name           string
		cfg            *execsvc.Config
		defs           []llm.ToolDefinition
		wantAgents     []string
		wantTools      []string
		wantModels     []string
		wantAgentTools map[string][]string
		wantErr        bool
	}{
		{
			name: "basic with names",
			cfg: &execsvc.Config{
				Default: execcfg.Defaults{Agent: "chat", Model: "gpt-4o"},
				Agent: &mcpcfg.Group[*agent.Agent]{
					Items: []*agent.Agent{{Identity: agent.Identity{ID: "chat", Name: "Chat"}}},
				},
				Model: &mcpcfg.Group[*modelprovider.Config]{
					Items: []*modelprovider.Config{{ID: "gpt-4o"}},
				},
			},
			defs:       []llm.ToolDefinition{{Name: "fs_read"}},
			wantAgents: []string{"chat"},
			wantTools:  []string{"fs_read"},
			wantModels: []string{"gpt-4o"},
		},
		{
			name: "agent tool matching",
			cfg: &execsvc.Config{
				Default: execcfg.Defaults{Agent: "Chat", Model: "gpt"},
				Agent: &mcpcfg.Group[*agent.Agent]{
					Items: []*agent.Agent{{
						Identity: agent.Identity{ID: "Chat", Name: "Chat"},
						Tool:     agent.Tool{Items: []*llm.Tool{{Pattern: "workflow-"}}},
					}},
				},
			},
			defs:           []llm.ToolDefinition{{Name: "workflow-run"}, {Name: "system_patch-apply"}},
			wantAgents:     []string{"Chat"},
			wantTools:      []string{"system_patch-apply", "workflow-run"},
			wantAgentTools: map[string][]string{"Chat": {"workflow-run"}},
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
			defs:       []llm.ToolDefinition{{Name: "b"}, {Name: "a"}},
			wantAgents: []string{"agentA"},
			wantTools:  []string{"a", "b"},
			wantModels: []string{"m1", "m2"},
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
		if tc.cfg != nil {
			assert.EqualValues(t, tc.cfg.Default.Agent, got.Defaults.Agent, tc.name)
			assert.EqualValues(t, tc.cfg.Default.Model, got.Defaults.Model, tc.name)
			assert.EqualValues(t, tc.cfg.Default.Embedder, got.Defaults.Embedder, tc.name)
		}
		if tc.wantAgents != nil {
			assert.EqualValues(t, tc.wantAgents, got.Agents, tc.name)
		}
		if tc.wantTools != nil {
			assert.EqualValues(t, tc.wantTools, got.Tools, tc.name)
		}
		if tc.wantModels != nil {
			assert.EqualValues(t, tc.wantModels, got.Models, tc.name)
		}
		if len(tc.wantAgentTools) > 0 {
			actualAgentTools := map[string][]string{}
			for id, info := range got.AgentInfo {
				if info != nil {
					actualAgentTools[id] = info.Tools
				}
			}
			assert.EqualValues(t, tc.wantAgentTools, actualAgentTools, tc.name)
		}
	}
}
