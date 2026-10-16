package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/viant/agently-core/app/executor"
	"github.com/viant/agently-core/genai/llm"
	mcpcfg "github.com/viant/agently-core/protocol/mcp/config"
	"github.com/viant/agently-core/protocol/mcp/manager"
	"github.com/viant/agently-core/protocol/tool"
	"github.com/viant/agently-core/workspace"
	"github.com/viant/jsonrpc"
	"github.com/viant/jsonrpc/transport"
	"github.com/viant/mcp"
	protocolclient "github.com/viant/mcp-protocol/client"
	"github.com/viant/mcp-protocol/logger"
	schema "github.com/viant/mcp-protocol/schema"
	protocolserver "github.com/viant/mcp-protocol/server"
	mcpclient "github.com/viant/mcp/client"
	mcpserver "github.com/viant/mcp/server"
)

type publicForecastProvider struct{ url string }

func (publicForecastProvider) Names(context.Context) ([]string, error) {
	return []string{"steward"}, nil
}
func (p publicForecastProvider) Options(context.Context, string) (*mcpcfg.MCPClient, error) {
	return &mcpcfg.MCPClient{ClientOptions: &mcp.ClientOptions{Name: "forecast-warmup", Version: "1", ProtocolVersion: "2025-11-25", Transport: mcp.ClientTransport{Type: "streamable", ClientTransportHTTP: mcp.ClientTransportHTTP{URL: p.url}}}, ToolsListVisibility: mcpcfg.ToolsListVisibilityPublic}, nil
}

type delayedForecastCatalog struct {
	mcpclient.Interface
	started, release chan struct{}
	once             sync.Once
	tool             schema.Tool
	failure          error
}

func (c *delayedForecastCatalog) ListTools(ctx context.Context, _ *string, _ ...mcpclient.RequestOption) (*schema.ListToolsResult, error) {
	c.once.Do(func() { close(c.started) })
	select {
	case <-c.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if c.failure != nil {
		return nil, c.failure
	}
	return &schema.ListToolsResult{Tools: []schema.Tool{c.tool}, CacheScope: schema.ListToolsResultCacheScopePublic, TtlMs: 60000}, nil
}

// Exercise the public catalog through the actual SDK transport. Custom client
// factories intentionally remain private regardless of a visibility hint.
type delayedForecastHandler struct {
	*protocolserver.DefaultHandler
	catalog *delayedForecastCatalog
}

func (h *delayedForecastHandler) Implements(method string) bool {
	return method == "tools/list" || h.DefaultHandler.Implements(method)
}
func (h *delayedForecastHandler) Initialize(ctx context.Context, in *schema.InitializeRequestParams, out *schema.InitializeResult) {
	h.DefaultHandler.Initialize(ctx, in, out)
	out.Capabilities.Tools = &schema.ServerCapabilitiesTools{}
}
func (h *delayedForecastHandler) Discover(ctx context.Context, out *schema.DiscoverResult) {
	h.DefaultHandler.Discover(ctx, out)
	out.Capabilities.Tools = &schema.ServerCapabilitiesTools{}
}
func (h *delayedForecastHandler) ListTools(ctx context.Context, _ *jsonrpc.TypedRequest[*schema.ListToolsRequest]) (*schema.ListToolsResult, *jsonrpc.Error) {
	result, err := h.catalog.ListTools(ctx, nil)
	if err != nil {
		return nil, jsonrpc.NewInternalError(err.Error(), nil)
	}
	return result, nil
}

type countedForecastRegistry struct {
	tool.Registry
	initializations atomic.Int32
}

func (r *countedForecastRegistry) InitializeWithRefreshContext(warmup, lifetime context.Context) <-chan struct{} {
	r.initializations.Add(1)
	return r.Registry.(interface {
		InitializeWithRefreshContext(context.Context, context.Context) <-chan struct{}
	}).InitializeWithRefreshContext(warmup, lifetime)
}
func (r *countedForecastRegistry) GetDefinitionWithContext(ctx context.Context, name string) (*llm.ToolDefinition, bool) {
	return r.Registry.(tool.ContextDefinitionGetter).GetDefinitionWithContext(ctx, name)
}

func realDelayedForecastRegistry(t *testing.T, failure error) (*executor.Runtime, *delayedForecastCatalog, *countedForecastRegistry) {
	t.Helper()
	t.Setenv("AGENTLY_MCP_SERVERS", "steward")
	previous := workspace.Root()
	workspace.SetRoot(t.TempDir())
	t.Cleanup(func() { workspace.SetRoot(previous) })
	definition := validForecastCapability()
	raw, err := json.Marshal(map[string]interface{}{"name": "ForecastingTargetingConvert", "inputSchema": definition.Parameters, "outputSchema": definition.OutputSchema})
	require.NoError(t, err)
	var entry schema.Tool
	require.NoError(t, json.Unmarshal(raw, &entry))
	catalog := &delayedForecastCatalog{started: make(chan struct{}), release: make(chan struct{}), tool: entry, failure: failure}
	server, err := mcpserver.New(mcpserver.WithImplementation(schema.Implementation{Name: "forecast-catalog", Version: "1"}), mcpserver.WithStreamableURI("/mcp"), mcpserver.WithRootRedirect(false), mcpserver.WithNewHandler(func(_ context.Context, n transport.Notifier, l logger.Logger, c protocolclient.Operations) (protocolserver.Handler, error) {
		return &delayedForecastHandler{DefaultHandler: protocolserver.NewDefaultHandler(n, l, c), catalog: catalog}, nil
	}))
	require.NoError(t, err)
	peer := httptest.NewServer(server.HTTP(context.Background(), "").Handler)
	t.Cleanup(func() { peer.CloseClientConnections(); peer.Close() })
	mgr, err := manager.New(publicForecastProvider{url: peer.URL + "/mcp"})
	require.NoError(t, err)
	t.Cleanup(func() { mgr.CloseConversation(""); mgr.CloseConversation("mcp-discovery:steward:background") })
	require.True(t, mgr.IsToolsListPublic(context.Background(), "steward"), "normal explicitly public HTTP catalog must be eligible for eager initialization")
	registry, err := tool.NewDefaultRegistry(mgr)
	require.NoError(t, err)
	counted := &countedForecastRegistry{Registry: registry}
	rt := &executor.Runtime{Registry: counted}
	t.Cleanup(func() { require.NoError(t, rt.Close(context.Background())) })
	return rt, catalog, counted
}
func TestForecastWarmupWaitsForExistingRealPublicCatalogBeforeCapability(t *testing.T) {
	rt, catalog, registry := realDelayedForecastRegistry(t, nil)
	_, present := registry.GetDefinitionWithContext(context.Background(), "steward/ForecastingTargetingConvert")
	require.False(t, present, "real public-cache miss must reproduce startup race")
	initial := rt.InitializeRegistryAsync(context.Background(), time.Second)
	select {
	case <-catalog.started:
	case <-time.After(time.Second):
		t.Fatal("catalog warmup did not start")
	}
	result := make(chan error, 1)
	go func() {
		if err := waitForecastRegistryWarmup(context.Background(), rt, time.Second); err != nil {
			result <- err
			return
		}
		result <- validateForecastEvidenceCapability(context.Background(), registry)
	}()
	select {
	case err := <-result:
		t.Fatalf("preflight finished before catalog completion: %v", err)
	case <-time.After(10 * time.Millisecond):
	}
	close(catalog.release)
	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("preflight did not finish")
	}
	require.Equal(t, int32(1), registry.initializations.Load(), "never duplicate initializer")
	require.Equal(t, initial, rt.InitializeRegistryAsync(context.Background(), time.Second))
}
func TestForecastWarmupFailureAndCancellationDoNotInventCapability(t *testing.T) {
	rt, catalog, registry := realDelayedForecastRegistry(t, errors.New("catalog unavailable"))
	rt.InitializeRegistryAsync(context.Background(), time.Second)
	select {
	case <-catalog.started:
	case <-time.After(time.Second):
		t.Fatal("warmup missing")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, waitForecastRegistryWarmup(ctx, rt, time.Second), context.Canceled)
	require.ErrorIs(t, waitForecastRegistryWarmup(context.Background(), rt, 5*time.Millisecond), context.DeadlineExceeded)
	close(catalog.release)
	require.NoError(t, waitForecastRegistryWarmup(context.Background(), rt, time.Second))
	require.Error(t, validateForecastEvidenceCapability(context.Background(), registry))
	require.Equal(t, int32(1), registry.initializations.Load())
}
func TestDisabledForecastWorkspaceDoesNotWaitOrStartWarmup(t *testing.T) {
	rt, _, registry := realDelayedForecastRegistry(t, nil)
	require.NoError(t, ConfigureForecastEvidence(context.Background(), rt, t.TempDir()))
	require.Zero(t, registry.initializations.Load())
}
