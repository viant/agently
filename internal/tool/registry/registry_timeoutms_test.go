package tool

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/internal/mcp/manager"
	mcpschema "github.com/viant/mcp-protocol/schema"
	mcpclient "github.com/viant/mcp/client"
)

func TestDefinitions_IncludesTimeoutMs(t *testing.T) {
	mgr, err := manager.New(nil)
	require.NoError(t, err)
	reg, err := NewWithManager(mgr)
	require.NoError(t, err)
	reg.replaceServerTools("db", []mcpschema.Tool{
		{
			Name: "ping",
			InputSchema: mcpschema.ToolInputSchema{
				Type: "object",
				Properties: mcpschema.ToolInputSchemaProperties(map[string]map[string]interface{}{
					"foo": {"type": "string"},
				}),
			},
		},
	})

	defs := reg.Definitions()
	var found *llm.ToolDefinition
	for i := range defs {
		if defs[i].Name == "db:ping" {
			found = &defs[i]
			break
		}
	}
	require.NotNil(t, found, "expected db:ping tool definition")
	props, _ := found.Parameters["properties"].(map[string]interface{})
	require.NotNil(t, props, "expected properties")
	_, ok := props[timeoutMsField]
	require.True(t, ok, "expected timeoutMs in properties")
}

func TestExecute_TimeoutMsInjected_StripsArgsAndSetsDeadline(t *testing.T) {
	var gotArgs map[string]interface{}
	var gotDeadline time.Time
	handler := func(ctx context.Context, args map[string]interface{}) (string, error) {
		gotArgs = args
		gotDeadline, _ = ctx.Deadline()
		return "ok", nil
	}
	reg := &Registry{
		cache: map[string]*toolCacheEntry{
			"db/ping": {
				def:            llm.ToolDefinition{Name: "db/ping"},
				exec:           handler,
				timeoutSupport: timeoutSupport{injected: true},
			},
		},
		virtualTimeout: map[string]timeoutSupport{},
	}

	_, err := reg.Execute(context.Background(), "db/ping", map[string]interface{}{
		"timeoutMs": 200.0,
		"foo":       "bar",
	})
	require.NoError(t, err)
	_, ok := gotArgs["timeoutMs"]
	require.False(t, ok, "expected timeoutMs to be stripped for injected schema")
	require.Equal(t, "bar", gotArgs["foo"])

	delta := time.Until(gotDeadline)
	require.Greater(t, delta, 50*time.Millisecond)
	require.Less(t, delta, time.Second)
}

func TestExecute_TimeoutMsNative_PreservesArgs(t *testing.T) {
	var gotArgs map[string]interface{}
	var gotDeadline time.Time
	handler := func(ctx context.Context, args map[string]interface{}) (string, error) {
		gotArgs = args
		gotDeadline, _ = ctx.Deadline()
		return "ok", nil
	}
	reg := &Registry{
		cache: map[string]*toolCacheEntry{
			"db/ping": {
				def:            llm.ToolDefinition{Name: "db/ping"},
				exec:           handler,
				timeoutSupport: timeoutSupport{native: true},
			},
		},
		virtualTimeout: map[string]timeoutSupport{},
	}

	_, err := reg.Execute(context.Background(), "db/ping", map[string]interface{}{
		"timeoutMs": int64(150),
		"foo":       "bar",
	})
	require.NoError(t, err)
	_, ok := gotArgs["timeoutMs"]
	require.True(t, ok, "expected timeoutMs to be preserved for native schema")
	require.Equal(t, "bar", gotArgs["foo"])

	delta := time.Until(gotDeadline)
	require.Greater(t, delta, 50*time.Millisecond)
	require.Less(t, delta, time.Second)
}

func TestEnsureTimeoutMs_SkipsWhenTimeoutSecPresent(t *testing.T) {
	def := &llm.ToolDefinition{
		Name: "svc/method",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"timeoutSec": map[string]interface{}{"type": "integer"},
			},
		},
	}
	support := ensureTimeoutMs(def)
	props := toolProperties(def)
	_, hasTimeoutMs := props[timeoutMsField]
	require.False(t, hasTimeoutMs, "timeoutMs should not be injected when timeoutSec exists")
	require.False(t, support.native)
	require.False(t, support.injected)
}

func TestEnsureTimeoutMs_SkipsWhenTimeoutPresent(t *testing.T) {
	def := &llm.ToolDefinition{
		Name: "svc/method",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"timeout": map[string]interface{}{"type": "string"},
			},
		},
	}
	support := ensureTimeoutMs(def)
	props := toolProperties(def)
	_, hasTimeoutMs := props[timeoutMsField]
	require.False(t, hasTimeoutMs, "timeoutMs should not be injected when timeout exists")
	require.False(t, support.native)
	require.False(t, support.injected)
}

func TestEnsureTimeoutMs_UsesNativeWhenTimeoutMsPresent(t *testing.T) {
	def := &llm.ToolDefinition{
		Name: "svc/method",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"timeoutMs": map[string]interface{}{"type": "integer"},
			},
		},
	}
	support := ensureTimeoutMs(def)
	props := toolProperties(def)
	_, hasTimeoutMs := props[timeoutMsField]
	require.True(t, hasTimeoutMs, "timeoutMs should remain when already present")
	require.True(t, support.native)
	require.False(t, support.injected)
}

func TestEnsureTimeoutMs_MergesWithoutOverridingProperties(t *testing.T) {
	def := &llm.ToolDefinition{
		Name: "svc/method",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"foo": map[string]interface{}{"type": "string"},
			},
		},
	}
	support := ensureTimeoutMs(def)
	props := toolProperties(def)
	require.Equal(t, "string", props["foo"].(map[string]interface{})["type"])
	_, hasTimeoutMs := props[timeoutMsField]
	require.True(t, hasTimeoutMs, "timeoutMs should be added")
	require.False(t, support.native)
	require.True(t, support.injected)
}

func TestEnsureTimeoutMs_PreservesMapStringMapSchema(t *testing.T) {
	def := &llm.ToolDefinition{
		Name: "svc/method",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]map[string]interface{}{
				"plan": {"type": "array"},
			},
		},
	}
	support := ensureTimeoutMs(def)
	props := toolProperties(def)
	_, hasPlan := props["plan"]
	require.True(t, hasPlan, "expected existing properties to remain")
	_, hasTimeoutMs := props[timeoutMsField]
	require.True(t, hasTimeoutMs, "timeoutMs should be added")
	require.False(t, support.native)
	require.True(t, support.injected)
}

func TestEnsureTimeoutMs_PreservesMCPInputSchemaProperties(t *testing.T) {
	def := &llm.ToolDefinition{
		Name: "svc/method",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": mcpschema.ToolInputSchemaProperties(map[string]map[string]interface{}{
				"plan": {"type": "array"},
			}),
		},
	}
	support := ensureTimeoutMs(def)
	props := toolProperties(def)
	_, hasPlan := props["plan"]
	require.True(t, hasPlan, "expected existing properties to remain")
	_, hasTimeoutMs := props[timeoutMsField]
	require.True(t, hasTimeoutMs, "timeoutMs should be added")
	require.False(t, support.native)
	require.True(t, support.injected)
}

func TestEnsureTimeoutMs_PreservesInterfaceMapProperties(t *testing.T) {
	def := &llm.ToolDefinition{
		Name: "svc/method",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[interface{}]interface{}{
				"plan": map[string]interface{}{"type": "array"},
			},
		},
	}
	support := ensureTimeoutMs(def)
	props := toolProperties(def)
	_, hasPlan := props["plan"]
	require.True(t, hasPlan, "expected existing properties to remain")
	_, hasTimeoutMs := props[timeoutMsField]
	require.True(t, hasTimeoutMs, "timeoutMs should be added")
	require.False(t, support.native)
	require.True(t, support.injected)
}

func TestDefinitions_SkipsTimeoutMsForInternalTools(t *testing.T) {
	mgr, err := manager.New(nil)
	require.NoError(t, err)
	reg, err := NewWithManager(mgr)
	require.NoError(t, err)
	reg.internal = map[string]mcpclient.Interface{
		"system/exec": &toolListClient{tools: []mcpschema.Tool{
			{
				Name: "execute",
				InputSchema: mcpschema.ToolInputSchema{
					Type: "object",
					Properties: mcpschema.ToolInputSchemaProperties(map[string]map[string]interface{}{
						"command": {"type": "string"},
					}),
				},
			},
		}},
	}

	defs := reg.Definitions()
	var def *llm.ToolDefinition
	for i := range defs {
		if defs[i].Name == "system/exec:execute" {
			def = &defs[i]
			break
		}
	}
	require.NotNil(t, def, "expected system/exec:execute tool definition")
	props, _ := def.Parameters["properties"].(map[string]interface{})
	require.NotNil(t, props)
	_, hasTimeoutMs := props[timeoutMsField]
	require.False(t, hasTimeoutMs, "timeoutMs should not be injected for internal tools")
	_, hasCommand := props["command"]
	require.True(t, hasCommand, "expected original properties to remain")
}

type toolListClient struct {
	tools []mcpschema.Tool
}

func (c *toolListClient) Initialize(ctx context.Context, options ...mcpclient.RequestOption) (*mcpschema.InitializeResult, error) {
	return &mcpschema.InitializeResult{}, nil
}
func (c *toolListClient) ListResourceTemplates(ctx context.Context, cursor *string, options ...mcpclient.RequestOption) (*mcpschema.ListResourceTemplatesResult, error) {
	return nil, errNotImplementedLocal
}
func (c *toolListClient) ListResources(ctx context.Context, cursor *string, options ...mcpclient.RequestOption) (*mcpschema.ListResourcesResult, error) {
	return nil, errNotImplementedLocal
}
func (c *toolListClient) ListPrompts(ctx context.Context, cursor *string, options ...mcpclient.RequestOption) (*mcpschema.ListPromptsResult, error) {
	return nil, errNotImplementedLocal
}
func (c *toolListClient) ListTools(ctx context.Context, cursor *string, options ...mcpclient.RequestOption) (*mcpschema.ListToolsResult, error) {
	return &mcpschema.ListToolsResult{Tools: c.tools}, nil
}
func (c *toolListClient) ReadResource(ctx context.Context, params *mcpschema.ReadResourceRequestParams, options ...mcpclient.RequestOption) (*mcpschema.ReadResourceResult, error) {
	return nil, errNotImplementedLocal
}
func (c *toolListClient) GetPrompt(ctx context.Context, params *mcpschema.GetPromptRequestParams, options ...mcpclient.RequestOption) (*mcpschema.GetPromptResult, error) {
	return nil, errNotImplementedLocal
}
func (c *toolListClient) CallTool(ctx context.Context, params *mcpschema.CallToolRequestParams, options ...mcpclient.RequestOption) (*mcpschema.CallToolResult, error) {
	return &mcpschema.CallToolResult{}, nil
}
func (c *toolListClient) Complete(ctx context.Context, params *mcpschema.CompleteRequestParams, options ...mcpclient.RequestOption) (*mcpschema.CompleteResult, error) {
	return nil, errNotImplementedLocal
}
func (c *toolListClient) Ping(ctx context.Context, params *mcpschema.PingRequestParams, options ...mcpclient.RequestOption) (*mcpschema.PingResult, error) {
	return nil, errNotImplementedLocal
}
func (c *toolListClient) Subscribe(ctx context.Context, params *mcpschema.SubscribeRequestParams, options ...mcpclient.RequestOption) (*mcpschema.SubscribeResult, error) {
	return nil, errNotImplementedLocal
}
func (c *toolListClient) Unsubscribe(ctx context.Context, params *mcpschema.UnsubscribeRequestParams, options ...mcpclient.RequestOption) (*mcpschema.UnsubscribeResult, error) {
	return nil, errNotImplementedLocal
}
func (c *toolListClient) SetLevel(ctx context.Context, params *mcpschema.SetLevelRequestParams, options ...mcpclient.RequestOption) (*mcpschema.SetLevelResult, error) {
	return nil, errNotImplementedLocal
}
func (c *toolListClient) ListRoots(ctx context.Context, params *mcpschema.ListRootsRequestParams, options ...mcpclient.RequestOption) (*mcpschema.ListRootsResult, error) {
	return nil, errNotImplementedLocal
}
func (c *toolListClient) CreateMessage(ctx context.Context, params *mcpschema.CreateMessageRequestParams, options ...mcpclient.RequestOption) (*mcpschema.CreateMessageResult, error) {
	return nil, errNotImplementedLocal
}
func (c *toolListClient) Elicit(ctx context.Context, params *mcpschema.ElicitRequestParams, options ...mcpclient.RequestOption) (*mcpschema.ElicitResult, error) {
	return nil, errNotImplementedLocal
}

var errNotImplementedLocal = errors.New("not implemented")
