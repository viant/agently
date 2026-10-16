package agently

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	uihttp "github.com/viant/agently-core/adapter/http/ui"
	"github.com/viant/agently-core/app/executor"
	"github.com/viant/agently-core/app/executor/config"
	appserver "github.com/viant/agently-core/app/server"
	mcpcfg "github.com/viant/agently-core/protocol/mcp/config"
	"github.com/viant/agently-core/protocol/mcp/manager"
	identity "github.com/viant/agently-core/protocol/resource"
	authsvc "github.com/viant/agently-core/service/auth"
	forge "github.com/viant/agently-core/service/primitiveprovider"
	"github.com/viant/forge/backend/types"
	"github.com/viant/mcp"
	mcpclient "github.com/viant/mcp/client"
	authcfg "github.com/viant/mcp/client/auth/config"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestDefaultFrameworkChatUsesSameNativeGatewayAndPreservesTargets(t *testing.T) {
	for _, key := range []string{"AGENTLY_DB_DRIVER", "AGENTLY_DB_DSN", "AGENTLY_DB_PATH", "AGENTLY_DB_SECRETS"} {
		t.Setenv(key, "")
	}
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "extension/forge"), 0700))
	entries, err := defaultFrameworkWindows()
	require.NoError(t, err)
	require.NotEmpty(t, entries)
	rt, _, _, err := appserver.BuildWorkspaceRuntime(context.Background(), appserver.RuntimeOptions{WorkspaceRoot: root, Defaults: &config.Defaults{}, SkipRegistryInitialize: true, ConfigureBuilder: func(_ context.Context, b *executor.Builder) error { b.WithNativeWindowDefaults(entries); return nil }})
	require.NoError(t, err)
	defer rt.Close(context.Background())
	require.True(t, rt.UIBridge.UsesWindowResourceResolution())
	got, err := rt.UIBridge.WindowDefinitionGet(context.Background(), &forge.WindowDefinitionGetInput{WindowID: "chat/new"})
	require.NoError(t, err)
	require.NotNil(t, got.Definition.Resource)
	require.Equal(t, "window://system/chat/new", got.Definition.Resource.URI)
	require.NotNil(t, got.Definition.View.Content)
}

type defaultFrameworkUnlinkedProvider struct{}

func (defaultFrameworkUnlinkedProvider) Names(context.Context) ([]string, error) {
	return []string{"asana"}, nil
}
func (defaultFrameworkUnlinkedProvider) Options(context.Context, string) (*mcpcfg.MCPClient, error) {
	return &mcpcfg.MCPClient{PrimitiveProviderIdentity: "asana", ClientOptions: &mcp.ClientOptions{Name: "optional-asana", Version: "1", Transport: mcp.ClientTransport{Type: "streamable", ClientTransportHTTP: mcp.ClientTransportHTTP{URL: "https://asana.example/mcp"}}}}, nil
}

func TestAuthenticatedDefaultFrameworkHTTPWindowsDoNotDiscoverUnlinkedConnector(t *testing.T) {
	for _, key := range []string{"AGENTLY_DB_DRIVER", "AGENTLY_DB_DSN", "AGENTLY_DB_PATH", "AGENTLY_DB_SECRETS"} {
		t.Setenv(key, "")
	}
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "extension/forge"), 0700))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "ui"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "config.yaml"), []byte("auth:\n  enabled: true\nui:\n  layout:\n    ref: ui/layout.yaml\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "ui/layout.yaml"), []byte("version: 1\nid: steward\napplications:\n  - id: steward\n    title: Workspace\n    menus:\n      - id: advertisers\n        title: Advertisers\n        action: {type: window, windowKey: advertiserList}\n"), 0600))
	var connectorCalls atomic.Int32
	linkRequired := &authcfg.OAuthLinkRequiredError{ServerName: "asana", ProviderRef: "asana-oauth", Resource: "https://asana.example/mcp"}
	mgr, err := manager.New(defaultFrameworkUnlinkedProvider{}, manager.WithClientFactory(func(_ context.Context, _ string, name string) (mcpclient.Interface, error) {
		connectorCalls.Add(1)
		require.Equal(t, "asana", name)
		return nil, linkRequired
	}))
	require.NoError(t, err)
	entries, err := defaultFrameworkWindows()
	require.NoError(t, err)
	rt, _, _, err := appserver.BuildWorkspaceRuntime(context.Background(), appserver.RuntimeOptions{WorkspaceRoot: root, Defaults: &config.Defaults{}, SkipRegistryInitialize: true, ConfigureBuilder: func(_ context.Context, b *executor.Builder) error {
		b.WithMCPManager(mgr).WithNativeWindowDefaults(entries)
		return nil
	}})
	require.NoError(t, err)
	defer rt.Close(context.Background())
	require.NotNil(t, rt.AuthConfig)
	require.True(t, rt.AuthConfig.Enabled)
	handler := uihttp.NewEmbeddedHandlerWithAuthorization("embed://localhost/window", nil, rt.AuthorizationPolicy, nil, uihttp.WithWindowResourceProvider(rt.UIBridge))
	// This context models the identity injected by verified ingress. The backend
	// provider's AuthEnabled gate still rejects anonymous requests below.
	ctx := authsvc.InjectUser(context.Background(), "signed-in-owner")
	for _, key := range []string{"chat/new", "chat/history", "schedule"} {
		t.Run(key, func(t *testing.T) {
			path := "/window/" + url.PathEscape(key) + "?platform=web&formFactor=desktop&surface=browser&capabilities=markdown&capabilities=chart&capabilities=upload&capabilities=code&capabilities=diff"
			request := httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			var envelope struct {
				Status string        `json:"status"`
				Data   *types.Window `json:"data"`
			}
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
			require.Equal(t, "ok", envelope.Status)
			require.NotNil(t, envelope.Data)
			window := envelope.Data
			require.NotNil(t, window.Resource)
			require.Equal(t, "window://system/"+key, window.Resource.URI)
			require.NotNil(t, window.View.Content)
			anonymous := httptest.NewRecorder()
			handler.ServeHTTP(anonymous, httptest.NewRequest(http.MethodGet, path, nil))
			require.NotEqual(t, http.StatusOK, anonymous.Code, "anonymous request must not bypass owning-provider authentication")
		})
	}
	require.Zero(t, connectorCalls.Load(), "trusted framework aliases must route directly to their declared owner")
	_, err = rt.PrimitiveProviders.GetForProvider(ctx, "asana", identity.ResourceRef{URI: "window://asana/project"}, nil)
	require.ErrorIs(t, err, linkRequired, "direct unlinked-connector access must retain its typed error")
	require.EqualValues(t, 1, connectorCalls.Load())
}
