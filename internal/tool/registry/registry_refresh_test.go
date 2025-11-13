package tool

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/viant/agently/internal/mcp/manager"
	mcpschema "github.com/viant/mcp-protocol/schema"
	mcpclient "github.com/viant/mcp/client"
)

// fakeMCPClient toggles between down and up for ListTools.
type fakeMCPClient struct {
	up atomic.Bool
}

func (f *fakeMCPClient) setUp(v bool) { f.up.Store(v) }

func (f *fakeMCPClient) Initialize(ctx context.Context, options ...mcpclient.RequestOption) (*mcpschema.InitializeResult, error) {
	return &mcpschema.InitializeResult{}, nil
}
func (f *fakeMCPClient) ListResourceTemplates(ctx context.Context, cursor *string, options ...mcpclient.RequestOption) (*mcpschema.ListResourceTemplatesResult, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeMCPClient) ListResources(ctx context.Context, cursor *string, options ...mcpclient.RequestOption) (*mcpschema.ListResourcesResult, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeMCPClient) ListPrompts(ctx context.Context, cursor *string, options ...mcpclient.RequestOption) (*mcpschema.ListPromptsResult, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeMCPClient) ListTools(ctx context.Context, cursor *string, options ...mcpclient.RequestOption) (*mcpschema.ListToolsResult, error) {
	if !f.up.Load() {
		return nil, errors.New("server down")
	}
	// One page only
	return &mcpschema.ListToolsResult{Tools: []mcpschema.Tool{{Name: "db/ping", Description: ptr("health check")}}}, nil
}
func (f *fakeMCPClient) ReadResource(ctx context.Context, params *mcpschema.ReadResourceRequestParams, options ...mcpclient.RequestOption) (*mcpschema.ReadResourceResult, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeMCPClient) GetPrompt(ctx context.Context, params *mcpschema.GetPromptRequestParams, options ...mcpclient.RequestOption) (*mcpschema.GetPromptResult, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeMCPClient) CallTool(ctx context.Context, params *mcpschema.CallToolRequestParams, options ...mcpclient.RequestOption) (*mcpschema.CallToolResult, error) {
	return &mcpschema.CallToolResult{}, nil
}
func (f *fakeMCPClient) Complete(ctx context.Context, params *mcpschema.CompleteRequestParams, options ...mcpclient.RequestOption) (*mcpschema.CompleteResult, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeMCPClient) Ping(ctx context.Context, params *mcpschema.PingRequestParams, options ...mcpclient.RequestOption) (*mcpschema.PingResult, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeMCPClient) Subscribe(ctx context.Context, params *mcpschema.SubscribeRequestParams, options ...mcpclient.RequestOption) (*mcpschema.SubscribeResult, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeMCPClient) Unsubscribe(ctx context.Context, params *mcpschema.UnsubscribeRequestParams, options ...mcpclient.RequestOption) (*mcpschema.UnsubscribeResult, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeMCPClient) SetLevel(ctx context.Context, params *mcpschema.SetLevelRequestParams, options ...mcpclient.RequestOption) (*mcpschema.SetLevelResult, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeMCPClient) ListRoots(ctx context.Context, params *mcpschema.ListRootsRequestParams, options ...mcpclient.RequestOption) (*mcpschema.ListRootsResult, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeMCPClient) CreateMessage(ctx context.Context, params *mcpschema.CreateMessageRequestParams, options ...mcpclient.RequestOption) (*mcpschema.CreateMessageResult, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeMCPClient) Elicit(ctx context.Context, params *mcpschema.ElicitRequestParams, options ...mcpclient.RequestOption) (*mcpschema.ElicitResult, error) {
	return nil, errors.New("not implemented")
}

func ptr[T any](v T) *T { return &v }

// TestAutoRegisterTools validates that when a server is initially down and comes back
// online, the registry auto-refresh registers its tools without restart.
func TestAutoRegisterTools(t *testing.T) {
	// Data-driven cases
	type testCase struct {
		name         string
		bringUpAfter time.Duration
		expectTool   string
	}
	cases := []testCase{
		{name: "up after 200ms", bringUpAfter: 200 * time.Millisecond, expectTool: "db/ping"},
		{name: "up after 1s", bringUpAfter: time.Second, expectTool: "db/ping"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Manager required by constructor but unused due to internal client path
			mgr, _ := manager.New(nil)
			reg, err := NewWithManager(mgr)
			if err != nil {
				t.Fatalf("registry init failed: %v", err)
			}
			// speed up refresh cadence
			reg.refreshEvery = 50 * time.Millisecond
			// inject fake internal MCP client under server name "db"
			fake := &fakeMCPClient{}
			fake.setUp(false) // start down
			reg.internal["db"] = fake

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// init will start auto-refresh monitoring
			reg.Initialize(ctx)

			// Schedule server to come back online
			time.AfterFunc(tc.bringUpAfter, func() { fake.setUp(true) })

			// Wait until tool shows up or timeout
			deadline := time.Now().Add(3 * time.Second)
			var defName string
			for time.Now().Before(deadline) {
				if d, ok := reg.GetDefinition("db/ping"); ok && d != nil {
					defName = d.Name
					break
				}
				time.Sleep(25 * time.Millisecond)
			}
			if defName == "" {
				t.Fatalf("tool not auto-registered: %s", tc.expectTool)
			}
			if defName != tc.expectTool {
				t.Fatalf("unexpected tool registered, expected %s, got %s", tc.expectTool, defName)
			}
		})
	}
}

// Minimal shim to avoid importing full genai/llm in test assertions
// no shim types needed
