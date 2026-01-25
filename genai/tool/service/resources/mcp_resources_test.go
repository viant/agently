package resources

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiconv "github.com/viant/agently/client/conversation"
	agmodel "github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/memory"
	mcpcfg "github.com/viant/agently/internal/mcp/config"
	mcpmgr "github.com/viant/agently/internal/mcp/manager"
	convmem "github.com/viant/agently/internal/service/conversation/memory"
)

type fakeMCPProvider struct {
	clients map[string]*mcpcfg.MCPClient
}

func (p *fakeMCPProvider) Options(_ context.Context, serverName string) (*mcpcfg.MCPClient, error) {
	if p == nil || p.clients == nil {
		return nil, fmt.Errorf("no provider")
	}
	if client, ok := p.clients[serverName]; ok {
		return client, nil
	}
	return nil, fmt.Errorf("server %q not found", serverName)
}

func mcpTestMetadata() map[string]interface{} {
	return map[string]interface{}{
		"resources": map[string]interface{}{
			"roots": []interface{}{
				map[string]interface{}{
					"id":            "mediator",
					"uri":           "mcp:github://github.vianttech.com/adelphic/mediator",
					"description":   "Mediator",
					"vectorization": true,
					"snapshot":      true,
					"allowGrep":     true,
				},
				map[string]interface{}{
					"id":            "mdp",
					"uri":           "mcp:github://github.vianttech.com/viant/mdp",
					"description":   "MDP",
					"vectorization": true,
					"snapshot":      true,
					"allowGrep":     false,
				},
			},
		},
	}
}

func TestRoots_MCPShorthand_SelectByID(t *testing.T) {
	agentID := "test-agent"
	convClient := convmem.New()
	conv := apiconv.NewConversation()
	conv.SetId("conv-1")
	conv.SetAgentId(agentID)
	require.NoError(t, convClient.PatchConversations(context.Background(), conv))

	mgr, err := mcpmgr.New(&fakeMCPProvider{
		clients: map[string]*mcpcfg.MCPClient{
			"github": {Metadata: mcpTestMetadata()},
		},
	})
	require.NoError(t, err)

	svc := New(nil,
		WithMCPManager(mgr),
		WithConversationClient(convClient),
		WithAgentFinder(&testAgentFinder{agent: &agmodel.Agent{
			Identity: agmodel.Identity{ID: agentID},
			Resources: []*agmodel.Resource{
				{MCP: "github", Roots: []string{"mediator"}, Role: "system"},
			},
		}}),
	)

	ctx := memory.WithConversationID(context.Background(), "conv-1")
	var out RootsOutput
	err = svc.roots(ctx, &RootsInput{}, &out)
	require.NoError(t, err)
	require.Len(t, out.Roots, 1)
	assert.Equal(t, "mediator", out.Roots[0].ID)
	assert.Equal(t, "mcp:github://github.vianttech.com/adelphic/mediator", out.Roots[0].URI)
	assert.Equal(t, "system", out.Roots[0].Role)
	assert.True(t, out.Roots[0].AllowedSemanticSearch)
	assert.True(t, out.Roots[0].AllowedGrepSearch)
}

func TestRoots_MCPShorthand_All(t *testing.T) {
	agentID := "test-agent"
	convClient := convmem.New()
	conv := apiconv.NewConversation()
	conv.SetId("conv-2")
	conv.SetAgentId(agentID)
	require.NoError(t, convClient.PatchConversations(context.Background(), conv))

	mgr, err := mcpmgr.New(&fakeMCPProvider{
		clients: map[string]*mcpcfg.MCPClient{
			"github": {Metadata: mcpTestMetadata()},
		},
	})
	require.NoError(t, err)

	svc := New(nil,
		WithMCPManager(mgr),
		WithConversationClient(convClient),
		WithAgentFinder(&testAgentFinder{agent: &agmodel.Agent{
			Identity: agmodel.Identity{ID: agentID},
			Resources: []*agmodel.Resource{
				{MCP: "github", Roots: []string{"*"}, Role: "user"},
			},
		}}),
	)

	ctx := memory.WithConversationID(context.Background(), "conv-2")
	var out RootsOutput
	err = svc.roots(ctx, &RootsInput{}, &out)
	require.NoError(t, err)
	require.Len(t, out.Roots, 2)
	ids := []string{out.Roots[0].ID, out.Roots[1].ID}
	assert.Contains(t, ids, "mediator")
	assert.Contains(t, ids, "mdp")
}
