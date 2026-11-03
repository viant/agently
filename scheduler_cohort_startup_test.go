package agently

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/viant/agently-core/app/executor"
	execconfig "github.com/viant/agently-core/app/executor/config"
	appserver "github.com/viant/agently-core/app/server"
	mcpcfg "github.com/viant/agently-core/protocol/mcp/config"
	"github.com/viant/agently-core/protocol/mcp/manager"
	"github.com/viant/agently-core/protocol/tool"
	"github.com/viant/agently-core/runtime/discovery"
	svcauth "github.com/viant/agently-core/service/auth"
	"github.com/viant/agently-core/workspace"
	primitive "github.com/viant/forge/backend/provider/contract"
	identity "github.com/viant/forge/backend/resource"
	"github.com/viant/forge/backend/types"
	"github.com/viant/jsonrpc"
	"github.com/viant/mcp"
	"github.com/viant/mcp-protocol/schema"
	mcpclient "github.com/viant/mcp/client"
)

func TestSchedulerLoadsConfiguredArtifactCohortsBeforeRunning(t *testing.T) {
	root := t.TempDir()
	previousRoot, previousRuntime := workspace.Root(), workspace.RuntimeRoot()
	t.Cleanup(func() { workspace.SetRoot(previousRoot); workspace.SetRuntimeRoot(previousRuntime) })
	for _, key := range []string{"AGENTLY_DB_DRIVER", "AGENTLY_DB_DSN", "AGENTLY_DB_PATH", "AGENTLY_DB_SECRETS", "AGENTLY_ARTIFACT_COHORT_CONFIG"} {
		t.Setenv(key, "")
	}
	t.Setenv("AGENTLY_WORKSPACE", root)
	t.Setenv("AGENTLY_RUNTIME_ROOT", filepath.Join(root, "runtime"))
	require.NoError(t, os.WriteFile(filepath.Join(root, "config.yaml"), []byte("artifactCohortConfig: cohorts.yaml\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "cohorts.yaml"), []byte("[]\n"), 0600))
	err := RunScheduler(SchedulerRunOptions{Once: true})
	require.ErrorContains(t, err, "artifact cohort", "scheduler startup must load the same configured metadata as the web host")
}

func TestSchedulerLoadsConfiguredUserFactsBeforeRunning(t *testing.T) {
	root := t.TempDir()
	previousRoot, previousRuntime := workspace.Root(), workspace.RuntimeRoot()
	t.Cleanup(func() { workspace.SetRoot(previousRoot); workspace.SetRuntimeRoot(previousRuntime) })
	for _, key := range []string{"AGENTLY_DB_DRIVER", "AGENTLY_DB_DSN", "AGENTLY_DB_PATH", "AGENTLY_DB_SECRETS", "AGENTLY_ARTIFACT_COHORT_CONFIG", "AGENTLY_AUTHZ_CONFIG"} {
		t.Setenv(key, "")
	}
	t.Setenv("AGENTLY_WORKSPACE", root)
	t.Setenv("AGENTLY_RUNTIME_ROOT", filepath.Join(root, "runtime"))
	require.NoError(t, os.WriteFile(filepath.Join(root, "config.yaml"), []byte("ui:\n  presentation:\n    identityFile: missing-identity.json\n"), 0600))
	err := RunScheduler(SchedulerRunOptions{Once: true})
	require.ErrorContains(t, err, "presentation identity configuration", "scheduled users must use the configured facts source, without requiring a browser")
}

type schedulerMetadataFixture struct {
	syncs   atomic.Int64
	syncErr error
}

func (s *schedulerMetadataFixture) Sync(context.Context) error         { s.syncs.Add(1); return s.syncErr }
func (*schedulerMetadataFixture) EnsureAdmitted(context.Context) error { return nil }
func (*schedulerMetadataFixture) OpenWindowVariant(context.Context, string, *types.WindowTarget) (*types.WindowResourceVariant, string, error) {
	return nil, "", identity.ErrResourceDenied
}
func (*schedulerMetadataFixture) AdmissionKey(context.Context) (string, error) { return "fixture", nil }
func (*schedulerMetadataFixture) CurrentHash(context.Context, string) (string, error) {
	return "fixture", nil
}
func (*schedulerMetadataFixture) Namespaces(context.Context) ([]primitive.NamespaceCapabilities, error) {
	return []primitive.NamespaceCapabilities{{ProviderIdentity: "producer", Namespace: "example"}}, nil
}
func (*schedulerMetadataFixture) List(context.Context, string, string) ([]primitive.ResourceState, error) {
	return nil, nil
}
func (*schedulerMetadataFixture) Get(context.Context, string) (*primitive.ResourceState, error) {
	return nil, identity.ErrResourceDenied
}

type schedulerToolClient struct{ mcpclient.Interface }

func TestArtifactCohortStartupFailureClosesPreparedSources(t *testing.T) {
	source := &schedulerMetadataFixture{syncErr: fmt.Errorf("fixture unavailable")}
	closed := 0
	_, closeCohorts, err := activateArtifactCohorts(context.Background(), context.Background(), nil, "", []ArtifactCohortBinding{{ProviderIdentity: "producer", Source: source, Close: func() { closed++ }}}, nil)
	require.ErrorContains(t, err, "artifact cohort preload")
	closeCohorts()
	require.Equal(t, 1, closed)
}

func (*schedulerToolClient) ListTools(context.Context, *string, ...mcpclient.RequestOption) (*schema.ListToolsResult, error) {
	return &schema.ListToolsResult{Tools: []schema.Tool{{Name: "read"}}}, nil
}

func TestSharedCohortStartupPreservesUserPublicAndPrivateCatalogs(t *testing.T) {
	for _, key := range []string{"AGENTLY_DB_DRIVER", "AGENTLY_DB_DSN", "AGENTLY_DB_PATH", "AGENTLY_DB_SECRETS"} {
		t.Setenv(key, "")
	}
	root := t.TempDir()
	previousRoot, previousRuntime := workspace.Root(), workspace.RuntimeRoot()
	t.Cleanup(func() { workspace.SetRoot(previousRoot); workspace.SetRuntimeRoot(previousRuntime) })
	require.NoError(t, os.WriteFile(filepath.Join(root, "config.yaml"), []byte("mcpCatalogs: [mcp://workspace/publisher]\n"), 0600))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr, err := manager.New(nil)
	require.NoError(t, err)
	var metadataCalls, publicCalls, privateCalls, browserCalls atomic.Int64
	for _, name := range []string{"publisher", "public-data", "private-data", "browser-device"} {
		options := &mcpcfg.MCPClient{ClientOptions: &mcp.ClientOptions{}, PrimitiveProviderIdentity: "producer", ToolsListVisibility: mcpcfg.ToolsListVisibilityPrivate}
		if name == "public-data" {
			options.ToolsListVisibility = mcpcfg.ToolsListVisibilityPublic
		}
		if name == "browser-device" {
			options.ExecutionLocation = "browser"
			options.BrowserTransport = &mcpcfg.BrowserTransport{Type: "chrome-extension", ExtensionID: strings.Repeat("a", 32), PortName: "fixture"}
		}
		require.NoError(t, mgr.RegisterLocal(ctx, name, options, func(callCtx context.Context) (mcpclient.Interface, error) {
			switch name {
			case "publisher":
				metadataCalls.Add(1)
				return nil, jsonrpc.NewHTTPStatusError(http.StatusForbidden)
			case "public-data":
				publicCalls.Add(1)
			case "private-data":
				privateCalls.Add(1)
				if svcauth.EffectiveUserID(callCtx) != "reader" {
					return nil, fmt.Errorf("private catalog lost its user")
				}
			case "browser-device":
				browserCalls.Add(1)
				return nil, fmt.Errorf("browser transport reached backend")
			}
			return &schedulerToolClient{}, nil
		}))
	}
	rt, _, _, err := appserver.BuildWorkspaceRuntime(ctx, appserver.RuntimeOptions{WorkspaceRoot: root, Defaults: &execconfig.Defaults{}, SkipRegistryInitialize: true, SchedulerHeadless: true, ConfigureBuilder: func(_ context.Context, b *executor.Builder) error { b.WithMCPManager(mgr); return nil }})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, rt.Close(context.Background())) })
	source := &schedulerMetadataFixture{}
	_, closeCohorts, err := activateArtifactCohorts(ctx, ctx, rt, "", []ArtifactCohortBinding{{ConnectionName: "mcp://workspace/publisher", ConnectionAliases: []string{"publisher"}, ProviderIdentity: "producer", Namespaces: []string{"example"}, Source: source, Metadata: source}}, nil)
	defer closeCohorts()
	require.NoError(t, err)
	require.EqualValues(t, 1, source.syncs.Load())
	lister, ok := rt.Registry.(tool.ContextDefinitionLister)
	require.True(t, ok)
	for _, scheduled := range []bool{true, false} {
		userCtx := discovery.WithMode(svcauth.InjectUser(ctx, "reader"), discovery.Mode{Scheduler: scheduled})
		definitions := lister.DefinitionsWithContext(userCtx)
		servers := map[string]bool{}
		for _, definition := range definitions {
			servers[definition.SourceMCPServer] = true
		}
		require.True(t, servers["public-data"], "public tools remain available (scheduled=%v)", scheduled)
		require.True(t, servers["private-data"], "private tools remain user-scoped (scheduled=%v)", scheduled)
		require.False(t, servers["publisher"], "metadata transport is not a tool bucket")
		require.False(t, servers["browser-device"], "headless execution has no browser tools")
	}
	require.Zero(t, metadataCalls.Load(), "catalog reads must use locally installed metadata")
	require.Zero(t, browserCalls.Load())
	require.Positive(t, publicCalls.Load())
	require.Positive(t, privateCalls.Load())
}
