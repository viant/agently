package agently

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	appserver "github.com/viant/agently-core/app/server"
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

func TestServedRuntime_ListTools_ExposesSkillTools(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "agents", "coder", "prompt"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "models"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "skills", "demo"), 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(root, "config.yaml"), []byte(`default:
  agent: coder
  model: openai_gpt-5.1
models:
  url: models
agents:
  url: agents
`), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(root, "models", "openai_gpt-5_1.yaml"), []byte(`id: openai_gpt-5.1
name: gpt-5.1 (OpenAI)
description: OpenAI gpt-5.1 model
options:
  provider: openai
  model: gpt-5.1
  envKey: OPENAI_API_KEY
`), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(root, "agents", "coder", "coder.yaml"), []byte(`id: coder
name: Coder
modelRef: openai_gpt-5.1
temperature: 0
prompt:
  uri: prompt/user.tmpl
systemPrompt:
  uri: prompt/system.tmpl
skills:
  - demo
tool:
  callExposure: conversation
  items:
    - name: llm/skills:list
    - name: llm/skills:activate
    - name: llm/agents:list
    - name: llm/agents:start
    - name: llm/agents:status
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "agents", "coder", "prompt", "user.tmpl"), []byte(`{{.Task.Prompt}}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "agents", "coder", "prompt", "system.tmpl"), []byte(`You are helpful.`), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(root, "skills", "demo", "SKILL.md"), []byte(`---
name: demo
description: Demo skill.
context: fork
---

# Demo
Use the demo skill.
`), 0o644))

	rt, _, _, err := appserver.BuildWorkspaceRuntime(context.Background(), appserver.RuntimeOptions{WorkspaceRoot: root})
	require.NoError(t, err)

	client, closeClient, err := sdk.NewLocalHTTPFromRuntime(context.Background(), rt)
	require.NoError(t, err)
	defer closeClient()

	defs, err := client.ListToolDefinitions(context.Background())
	require.NoError(t, err)

	var names []string
	for _, item := range defs {
		names = append(names, item.Name)
	}
	require.Contains(t, names, "llm/skills:list")
	require.Contains(t, names, "llm/skills:activate")
	require.Contains(t, names, "llm/agents:start")
	require.Contains(t, names, "llm/agents:status")
}
