package tool

import (
	"testing"

	"github.com/stretchr/testify/assert"
	coreagent "github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/llm"
	mcpproto "github.com/viant/mcp-protocol/schema"
)

func TestInjectVirtualAgentTools_ElicitationPropagates(t *testing.T) {
	// Given an agent published in the catalog with an elicitation block
	ag := &coreagent.Agent{
		Identity:    coreagent.Identity{ID: "demo"},
		Profile:     &coreagent.Profile{Publish: true},
		Description: "Demo agent",
		ContextInputs: &coreagent.ContextInputs{
			Enabled: true,
			ElicitRequestParams: mcpproto.ElicitRequestParams{
				Message: "Please provide demo payload",
				RequestedSchema: mcpproto.ElicitRequestParamsRequestedSchema{
					Type: "object",
					Properties: map[string]interface{}{
						"project": map[string]interface{}{"type": "string", "description": "Project name"},
					},
					Required: []string{"project"},
				},
			},
		},
	}

	r := &Registry{virtualDefs: map[string]llm.ToolDefinition{}, virtualExec: map[string]Handler{}}

	// When injecting virtual tools
	r.InjectVirtualAgentTools([]*coreagent.Agent{ag}, "")

	// Then the derived tool definition contains elicitation section in description
	def, ok := r.virtualDefs["agentExec/demo"]
	assert.True(t, ok, "virtual tool definition not found")
	// Description should contain an Elicitation Inputs section with 'project'
	assert.Contains(t, def.Description, "args.context")
	assert.Contains(t, def.Description, "context.project")
	assert.Contains(t, def.Description, "required")
}

// llmToolDefinitionStub exists to satisfy the type in a minimal way for testing the map shape only.
// no extra stubs needed
