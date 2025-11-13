package tool

import (
	"context"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/internal/mcp/manager"
	mcpschema "github.com/viant/mcp-protocol/schema"
	mcpclient "github.com/viant/mcp/client"
)

// fakeMCPClient supports modes for ListTools behavior.
type fakeClient struct{ mode string }

func (f *fakeClient) Initialize(ctx context.Context, options ...mcpclient.RequestOption) (*mcpschema.InitializeResult, error) {
	return &mcpschema.InitializeResult{}, nil
}
func (f *fakeClient) ListResourceTemplates(ctx context.Context, cursor *string, options ...mcpclient.RequestOption) (*mcpschema.ListResourceTemplatesResult, error) {
	return nil, errNotImplemented
}
func (f *fakeClient) ListResources(ctx context.Context, cursor *string, options ...mcpclient.RequestOption) (*mcpschema.ListResourcesResult, error) {
	return nil, errNotImplemented
}
func (f *fakeClient) ListPrompts(ctx context.Context, cursor *string, options ...mcpclient.RequestOption) (*mcpschema.ListPromptsResult, error) {
	return nil, errNotImplemented
}
func (f *fakeClient) ListTools(ctx context.Context, cursor *string, options ...mcpclient.RequestOption) (*mcpschema.ListToolsResult, error) {
	switch f.mode {
	case "down":
		return nil, errServerDown
	case "empty":
		return &mcpschema.ListToolsResult{Tools: []mcpschema.Tool{}}, nil
	default:
		return &mcpschema.ListToolsResult{Tools: []mcpschema.Tool{{Name: "ping"}}}, nil
	}
}
func (f *fakeClient) ReadResource(ctx context.Context, params *mcpschema.ReadResourceRequestParams, options ...mcpclient.RequestOption) (*mcpschema.ReadResourceResult, error) {
	return nil, errNotImplemented
}
func (f *fakeClient) GetPrompt(ctx context.Context, params *mcpschema.GetPromptRequestParams, options ...mcpclient.RequestOption) (*mcpschema.GetPromptResult, error) {
	return nil, errNotImplemented
}
func (f *fakeClient) CallTool(ctx context.Context, params *mcpschema.CallToolRequestParams, options ...mcpclient.RequestOption) (*mcpschema.CallToolResult, error) {
	return &mcpschema.CallToolResult{}, nil
}
func (f *fakeClient) Complete(ctx context.Context, params *mcpschema.CompleteRequestParams, options ...mcpclient.RequestOption) (*mcpschema.CompleteResult, error) {
	return nil, errNotImplemented
}
func (f *fakeClient) Ping(ctx context.Context, params *mcpschema.PingRequestParams, options ...mcpclient.RequestOption) (*mcpschema.PingResult, error) {
	return nil, errNotImplemented
}
func (f *fakeClient) Subscribe(ctx context.Context, params *mcpschema.SubscribeRequestParams, options ...mcpclient.RequestOption) (*mcpschema.SubscribeResult, error) {
	return nil, errNotImplemented
}
func (f *fakeClient) Unsubscribe(ctx context.Context, params *mcpschema.UnsubscribeRequestParams, options ...mcpclient.RequestOption) (*mcpschema.UnsubscribeResult, error) {
	return nil, errNotImplemented
}
func (f *fakeClient) SetLevel(ctx context.Context, params *mcpschema.SetLevelRequestParams, options ...mcpclient.RequestOption) (*mcpschema.SetLevelResult, error) {
	return nil, errNotImplemented
}
func (f *fakeClient) ListRoots(ctx context.Context, params *mcpschema.ListRootsRequestParams, options ...mcpclient.RequestOption) (*mcpschema.ListRootsResult, error) {
	return nil, errNotImplemented
}
func (f *fakeClient) CreateMessage(ctx context.Context, params *mcpschema.CreateMessageRequestParams, options ...mcpclient.RequestOption) (*mcpschema.CreateMessageResult, error) {
	return nil, errNotImplemented
}
func (f *fakeClient) Elicit(ctx context.Context, params *mcpschema.ElicitRequestParams, options ...mcpclient.RequestOption) (*mcpschema.ElicitResult, error) {
	return nil, errNotImplemented
}

// Errors used by the fake client.
var (
	errNotImplemented = &fakeError{"not implemented"}
	errServerDown     = &fakeError{"server down"}
)

type fakeError struct{ s string }

func (e *fakeError) Error() string { return e.s }

// Test that Definitions returns cached tools when server is down or empty.
func TestDefinitions_UsesCacheOnFailure(t *testing.T) {
	type testCase struct {
		name      string
		seedCache bool
		mode      string // down | empty | up
		expected  []string
	}
	cases := []testCase{
		{name: "down with cache", seedCache: true, mode: "down", expected: []string{"db:ping", "llm/exec:run_agent"}},
		{name: "down without cache", seedCache: false, mode: "down", expected: []string{"llm/exec:run_agent"}},
		{name: "empty with cache", seedCache: true, mode: "empty", expected: []string{"db:ping", "llm/exec:run_agent"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mgr, _ := manager.New(nil)
			reg, err := NewWithManager(mgr)
			if err != nil {
				t.Fatalf("registry init failed: %v", err)
			}
			// Limit internal servers to only our fake one for determinism.
			reg.internal = map[string]mcpclient.Interface{"db": &fakeClient{mode: tc.mode}}

			if tc.seedCache {
				// Seed cache with one tool for db
				reg.replaceServerTools("db", []mcpschema.Tool{{Name: "ping"}})
			}

			defs := reg.Definitions()
			names := make([]string, 0, len(defs))
			for _, d := range defs {
				names = append(names, d.Name)
			}
			sort.Strings(names)
			sort.Strings(tc.expected)
			assert.EqualValues(t, tc.expected, names)
		})
	}
}
