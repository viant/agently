package runtime

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/viant/mcp"
	schema "github.com/viant/mcp-protocol/schema"
	mcpclient "github.com/viant/mcp/client"
)

type publicForecastProvider struct{}

func (publicForecastProvider) Names(context.Context) ([]string, error) {
	return []string{"steward"}, nil
}
func (publicForecastProvider) Options(context.Context, string) (*mcpcfg.MCPClient, error) {
	return &mcpcfg.MCPClient{ClientOptions: &mcp.ClientOptions{}, ToolsListVisibility: mcpcfg.ToolsListVisibilityPublic}, nil
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
	return &schema.ListToolsResult{Tools: []schema.Tool{c.tool}}, nil
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
	mgr, err := manager.New(publicForecastProvider{}, manager.WithClientFactory(func(context.Context, string, string) (mcpclient.Interface, error) { return catalog, nil }))
	require.NoError(t, err)
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
