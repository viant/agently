package tool

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/viant/agently/genai/memory"
	mcpcfg "github.com/viant/agently/internal/mcp/config"
	mcpschema "github.com/viant/mcp-protocol/schema"
	mcpclient "github.com/viant/mcp/client"
)

func TestListServerTools_UsesConversationIDAsDiscoveryScope(t *testing.T) {
	stub := &discoveryManagerStub{
		getFunc: func(convID, server string) (mcpclient.Interface, error) {
			switch convID {
			case "conv-1":
				return &discoveryListClient{tools: []mcpschema.Tool{{Name: "alpha"}}}, nil
			case "conv-2":
				return &discoveryListClient{tools: []mcpschema.Tool{{Name: "beta"}}}, nil
			default:
				return nil, fmt.Errorf("unexpected discovery scope %q for server %q", convID, server)
			}
		},
	}
	reg := &Registry{mgr: stub}

	tools1, err := reg.listServerTools(memory.WithConversationID(context.Background(), "conv-1"), "guardian")
	if err != nil {
		t.Fatalf("listServerTools(conv-1) error: %v", err)
	}
	if len(tools1) != 1 || tools1[0].Name != "alpha" {
		t.Fatalf("unexpected tools for conv-1: %+v", tools1)
	}

	tools2, err := reg.listServerTools(memory.WithConversationID(context.Background(), "conv-2"), "guardian")
	if err != nil {
		t.Fatalf("listServerTools(conv-2) error: %v", err)
	}
	if len(tools2) != 1 || tools2[0].Name != "beta" {
		t.Fatalf("unexpected tools for conv-2: %+v", tools2)
	}

	getCalls := stub.getCallsSnapshot()
	if len(getCalls) != 2 {
		t.Fatalf("expected 2 manager Get calls, got %d", len(getCalls))
	}
	if getCalls[0].convID != "conv-1" || getCalls[1].convID != "conv-2" {
		t.Fatalf("expected conversation scopes [conv-1 conv-2], got %+v", getCalls)
	}
	for _, call := range getCalls {
		if call.server != "guardian" {
			t.Fatalf("expected guardian server for every Get call, got %+v", getCalls)
		}
		if call.convID == "" {
			t.Fatalf("expected non-empty discovery scope, got %+v", getCalls)
		}
	}
}

func TestListServerTools_UsesFreshSyntheticScopeWithoutConversationID(t *testing.T) {
	stub := &discoveryManagerStub{
		getFunc: func(convID, server string) (mcpclient.Interface, error) {
			if convID == "" {
				return nil, errors.New("empty discovery scope")
			}
			if !strings.HasPrefix(convID, "mcp-discovery:guardian:") {
				return nil, fmt.Errorf("unexpected synthetic scope %q", convID)
			}
			return &discoveryListClient{tools: []mcpschema.Tool{{Name: convID}}}, nil
		},
	}
	reg := &Registry{mgr: stub}

	first, err := reg.listServerTools(context.Background(), "guardian")
	if err != nil {
		t.Fatalf("first listServerTools() error: %v", err)
	}
	second, err := reg.listServerTools(context.Background(), "guardian")
	if err != nil {
		t.Fatalf("second listServerTools() error: %v", err)
	}

	getCalls := stub.getCallsSnapshot()
	if len(getCalls) != 2 {
		t.Fatalf("expected 2 manager Get calls, got %d", len(getCalls))
	}
	if getCalls[0].convID == "" || getCalls[1].convID == "" {
		t.Fatalf("expected synthetic discovery scopes, got %+v", getCalls)
	}
	if getCalls[0].convID == getCalls[1].convID {
		t.Fatalf("expected a fresh synthetic scope per discovery call, got %+v", getCalls)
	}
	if first[0].Name != getCalls[0].convID || second[0].Name != getCalls[1].convID {
		t.Fatalf("unexpected tool mapping for synthetic scopes: tools1=%+v tools2=%+v calls=%+v", first, second, getCalls)
	}
}

func TestListServerTools_RetryReusesSyntheticDiscoveryScope(t *testing.T) {
	var (
		getCount       int
		reconnectCount int
		syntheticScope string
	)
	stub := &discoveryManagerStub{
		getFunc: func(convID, server string) (mcpclient.Interface, error) {
			if convID == "" {
				return nil, errors.New("empty discovery scope")
			}
			if !strings.HasPrefix(convID, "mcp-discovery:guardian:") {
				return nil, fmt.Errorf("unexpected synthetic scope %q", convID)
			}
			if syntheticScope == "" {
				syntheticScope = convID
			} else if syntheticScope != convID {
				return nil, fmt.Errorf("retry used a different discovery scope: want %q got %q", syntheticScope, convID)
			}
			getCount++
			if getCount == 1 {
				return &discoveryListClient{listErr: errors.New("EOF")}, nil
			}
			return &discoveryListClient{tools: []mcpschema.Tool{{Name: "recovered"}}}, nil
		},
		reconnectFunc: func(convID, server string) (mcpclient.Interface, error) {
			reconnectCount++
			if syntheticScope != "" && convID != syntheticScope {
				return nil, fmt.Errorf("unexpected reconnect scope %q", convID)
			}
			return &discoveryListClient{tools: []mcpschema.Tool{{Name: "recovered"}}}, nil
		},
	}
	reg := &Registry{mgr: stub}

	tools, err := reg.listServerTools(context.Background(), "guardian")
	if err != nil {
		t.Fatalf("listServerTools() error: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "recovered" {
		t.Fatalf("unexpected tools after retry: %+v", tools)
	}
	if syntheticScope == "" {
		t.Fatal("expected retry path to use a synthetic discovery scope")
	}
	if reconnectCount != 1 {
		t.Fatalf("expected 1 reconnect, got %d", reconnectCount)
	}

	getCalls := stub.getCallsSnapshot()
	reconnectCalls := stub.reconnectCallsSnapshot()
	if len(getCalls) != 2 {
		t.Fatalf("expected 2 manager Get calls, got %d", len(getCalls))
	}
	if len(reconnectCalls) != 1 {
		t.Fatalf("expected 1 manager Reconnect call, got %d", len(reconnectCalls))
	}
	if getCalls[0].convID != syntheticScope || getCalls[1].convID != syntheticScope || reconnectCalls[0].convID != syntheticScope {
		t.Fatalf("expected retry to reuse scope %q, got gets=%+v reconnects=%+v", syntheticScope, getCalls, reconnectCalls)
	}
}

type discoveryManagerStub struct {
	mu             sync.Mutex
	getCalls       []discoveryManagerCall
	reconnectCalls []discoveryManagerCall
	getFunc        func(convID, server string) (mcpclient.Interface, error)
	reconnectFunc  func(convID, server string) (mcpclient.Interface, error)
}

type discoveryManagerCall struct {
	convID string
	server string
}

func (m *discoveryManagerStub) Get(_ context.Context, convID, serverName string) (mcpclient.Interface, error) {
	m.mu.Lock()
	m.getCalls = append(m.getCalls, discoveryManagerCall{convID: convID, server: serverName})
	fn := m.getFunc
	m.mu.Unlock()
	if fn == nil {
		return nil, fmt.Errorf("unexpected Get(%q, %q)", convID, serverName)
	}
	return fn(convID, serverName)
}

func (m *discoveryManagerStub) Reconnect(_ context.Context, convID, serverName string) (mcpclient.Interface, error) {
	m.mu.Lock()
	m.reconnectCalls = append(m.reconnectCalls, discoveryManagerCall{convID: convID, server: serverName})
	fn := m.reconnectFunc
	m.mu.Unlock()
	if fn == nil {
		return nil, fmt.Errorf("unexpected Reconnect(%q, %q)", convID, serverName)
	}
	return fn(convID, serverName)
}

func (m *discoveryManagerStub) Touch(convID, serverName string) {}

func (m *discoveryManagerStub) Options(ctx context.Context, serverName string) (*mcpcfg.MCPClient, error) {
	return nil, nil
}

func (m *discoveryManagerStub) UseIDToken(ctx context.Context, serverName string) bool {
	return false
}

func (m *discoveryManagerStub) WithAuthTokenContext(ctx context.Context, serverName string) context.Context {
	return ctx
}

func (m *discoveryManagerStub) getCallsSnapshot() []discoveryManagerCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	ret := make([]discoveryManagerCall, len(m.getCalls))
	copy(ret, m.getCalls)
	return ret
}

func (m *discoveryManagerStub) reconnectCallsSnapshot() []discoveryManagerCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	ret := make([]discoveryManagerCall, len(m.reconnectCalls))
	copy(ret, m.reconnectCalls)
	return ret
}

type discoveryListClient struct {
	tools   []mcpschema.Tool
	listErr error
}

func (c *discoveryListClient) Initialize(ctx context.Context, options ...mcpclient.RequestOption) (*mcpschema.InitializeResult, error) {
	return &mcpschema.InitializeResult{}, nil
}

func (c *discoveryListClient) ListResourceTemplates(ctx context.Context, cursor *string, options ...mcpclient.RequestOption) (*mcpschema.ListResourceTemplatesResult, error) {
	return nil, errDiscoveryNotImplemented
}

func (c *discoveryListClient) ListResources(ctx context.Context, cursor *string, options ...mcpclient.RequestOption) (*mcpschema.ListResourcesResult, error) {
	return nil, errDiscoveryNotImplemented
}

func (c *discoveryListClient) ListPrompts(ctx context.Context, cursor *string, options ...mcpclient.RequestOption) (*mcpschema.ListPromptsResult, error) {
	return nil, errDiscoveryNotImplemented
}

func (c *discoveryListClient) ListTools(ctx context.Context, cursor *string, options ...mcpclient.RequestOption) (*mcpschema.ListToolsResult, error) {
	if c.listErr != nil {
		return nil, c.listErr
	}
	return &mcpschema.ListToolsResult{Tools: c.tools}, nil
}

func (c *discoveryListClient) ReadResource(ctx context.Context, params *mcpschema.ReadResourceRequestParams, options ...mcpclient.RequestOption) (*mcpschema.ReadResourceResult, error) {
	return nil, errDiscoveryNotImplemented
}

func (c *discoveryListClient) GetPrompt(ctx context.Context, params *mcpschema.GetPromptRequestParams, options ...mcpclient.RequestOption) (*mcpschema.GetPromptResult, error) {
	return nil, errDiscoveryNotImplemented
}

func (c *discoveryListClient) CallTool(ctx context.Context, params *mcpschema.CallToolRequestParams, options ...mcpclient.RequestOption) (*mcpschema.CallToolResult, error) {
	return &mcpschema.CallToolResult{}, nil
}

func (c *discoveryListClient) Complete(ctx context.Context, params *mcpschema.CompleteRequestParams, options ...mcpclient.RequestOption) (*mcpschema.CompleteResult, error) {
	return nil, errDiscoveryNotImplemented
}

func (c *discoveryListClient) Ping(ctx context.Context, params *mcpschema.PingRequestParams, options ...mcpclient.RequestOption) (*mcpschema.PingResult, error) {
	return nil, errDiscoveryNotImplemented
}

func (c *discoveryListClient) Subscribe(ctx context.Context, params *mcpschema.SubscribeRequestParams, options ...mcpclient.RequestOption) (*mcpschema.SubscribeResult, error) {
	return nil, errDiscoveryNotImplemented
}

func (c *discoveryListClient) Unsubscribe(ctx context.Context, params *mcpschema.UnsubscribeRequestParams, options ...mcpclient.RequestOption) (*mcpschema.UnsubscribeResult, error) {
	return nil, errDiscoveryNotImplemented
}

func (c *discoveryListClient) SetLevel(ctx context.Context, params *mcpschema.SetLevelRequestParams, options ...mcpclient.RequestOption) (*mcpschema.SetLevelResult, error) {
	return nil, errDiscoveryNotImplemented
}

func (c *discoveryListClient) ListRoots(ctx context.Context, params *mcpschema.ListRootsRequestParams, options ...mcpclient.RequestOption) (*mcpschema.ListRootsResult, error) {
	return nil, errDiscoveryNotImplemented
}

func (c *discoveryListClient) CreateMessage(ctx context.Context, params *mcpschema.CreateMessageRequestParams, options ...mcpclient.RequestOption) (*mcpschema.CreateMessageResult, error) {
	return nil, errDiscoveryNotImplemented
}

func (c *discoveryListClient) Elicit(ctx context.Context, params *mcpschema.ElicitRequestParams, options ...mcpclient.RequestOption) (*mcpschema.ElicitResult, error) {
	return nil, errDiscoveryNotImplemented
}

var errDiscoveryNotImplemented = errors.New("not implemented")
