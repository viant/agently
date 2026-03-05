package tool

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/viant/agently/genai/llm"
	mcpclient "github.com/viant/mcp/client"
)

func TestRegistry_MatchDefinitionWithContext_ExplicitVirtualSkipsMCPDiscovery(t *testing.T) {
	r := &Registry{
		virtualDefs: map[string]llm.ToolDefinition{
			"llm/agents:list": {Name: "llm/agents:list", Description: "list workspace agents"},
		},
		virtualExec: map[string]Handler{},
		cache:       map[string]*toolCacheEntry{},
		internal:    map[string]mcpclient.Interface{},
	}

	defs := r.MatchDefinitionWithContext(context.Background(), "llm/agents:list")
	require.Len(t, defs, 1)
	require.Equal(t, "llm/agents:list", defs[0].Name)
	require.Empty(t, r.LastWarnings())
}
