package agently

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	toolschema "github.com/viant/agently-core/protocol/tool/schema"
	"github.com/viant/agently-core/sdk"
)

func TestFilterToolDefinitions_ByServiceNamespace(t *testing.T) {
	defs := []sdk.ToolDefinitionInfo{
		{Name: "system/os:getEnv"},
		{Name: "system/exec:execute"},
		{Name: "resources:list"},
		{Name: "llm/agents:run"},
	}

	got := filterToolDefinitions(defs, "system/os")
	require.Len(t, got, 1)
	require.Equal(t, "system/os:getEnv", got[0].Name)

	got = filterToolDefinitions(defs, "llm/agents")
	require.Len(t, got, 1)
	require.Equal(t, "llm/agents:run", got[0].Name)
}

func TestFilterToolDefinitions_AcceptsCanonicalServiceVariants(t *testing.T) {
	defs := []sdk.ToolDefinitionInfo{
		{Name: "system/os:getEnv"},
		{Name: "system/exec.execute"},
	}

	got := filterToolDefinitions(defs, "system_os")
	require.Len(t, got, 1)
	require.Equal(t, "system/os:getEnv", got[0].Name)

	got = filterToolDefinitions(defs, "system/exec")
	require.Len(t, got, 1)
	require.Equal(t, "system/exec.execute", got[0].Name)
}

func TestListToolsCmd_ResolveBaseURL_UsesAPIOverride(t *testing.T) {
	got, err := resolveToolBaseURL(context.Background(), "http://example:8080")
	require.NoError(t, err)
	require.Equal(t, "http://example:8080", got)
}

func TestMCPRunCmd_RequiresName(t *testing.T) {
	cmd := &MCPRunCmd{}
	err := cmd.Execute(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "--name is required")
}

func TestBuildExampleRequest_UsesSchemaDefaultsAndRequired(t *testing.T) {
	def := sdk.ToolDefinitionInfo{
		Name: "resources:read",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"uri": map[string]interface{}{
					"type": "string",
				},
				"encoding": map[string]interface{}{
					"type":    "string",
					"default": "utf-8",
				},
				"mode": map[string]interface{}{
					"type": "string",
					"enum": []interface{}{"fast", "full"},
				},
				"limit": map[string]interface{}{
					"type": "integer",
				},
			},
		},
		Required: []string{"uri"},
	}

	got := buildExampleRequest(def)
	asMap, ok := got.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, " ", asMap["uri"])
	require.Equal(t, "utf-8", asMap["encoding"])
	require.Equal(t, "fast", asMap["mode"])
	require.EqualValues(t, 1, asMap["limit"])
}

func TestBuildExampleRequest_UsesFormatAwarePlaceholders(t *testing.T) {
	def := sdk.ToolDefinitionInfo{
		Name: "forecasting:Total",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"From": map[string]interface{}{
					"type":   "string",
					"format": "date-time",
				},
				"Email": map[string]interface{}{
					"type":   "string",
					"format": "email",
				},
				"Callback": map[string]interface{}{
					"type":   "string",
					"format": "uri",
				},
			},
		},
		Required: []string{"From"},
	}

	got := buildExampleRequest(def)
	asMap, ok := got.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "2026-01-01T00:00:00Z", asMap["From"])
	require.Equal(t, "user@example.com", asMap["Email"])
	require.Equal(t, "https://example.com", asMap["Callback"])
}

func TestRenderSchema_DefaultsToGoShape(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"From": map[string]interface{}{
				"type":   "string",
				"format": "date-time",
			},
			"To": map[string]interface{}{
				"type": "string",
			},
		},
	}
	got := renderSchema(schema, "go")
	expected, err := toolschema.GoShapeFromSchemaMap(schema)
	require.NoError(t, err)
	require.Equal(t, expected, got)
	require.Contains(t, got, "time.Time")
	require.Contains(t, got, "From")
	require.Contains(t, got, "To")
	require.Contains(t, got, "\n")
}

func TestRenderSchema_JSONFormat(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"From": map[string]interface{}{"type": "string"},
		},
	}
	got := renderSchema(schema, "json")
	require.Contains(t, got, `"type":"object"`)
	require.Contains(t, got, `"From"`)
	require.NotContains(t, got, "struct {")
}

func TestToolNameCandidates_IncludeCommonVariants(t *testing.T) {
	got := toolNameCandidates("forecasting:Total")
	require.Contains(t, got, "forecasting:total")
	require.Contains(t, got, "forecasting/total")
	require.Contains(t, got, "forecasting-total")
}

func TestExecutableToolName_ColonToSlash(t *testing.T) {
	require.Equal(t, "forecasting/Total", executableToolName("forecasting:Total"))
	require.Equal(t, "forecasting/Total", executableToolName("forecasting/Total"))
}
