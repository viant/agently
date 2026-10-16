package agently

import (
	"context"
	"encoding/json"
	hostruntime "github.com/viant/agently-core/app/host/runtime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	uihttp "github.com/viant/agently-core/adapter/http/ui"
	"github.com/viant/agently-core/app/executor"
	"github.com/viant/agently-core/app/executor/config"
	appserver "github.com/viant/agently-core/app/server"
	svcworkspace "github.com/viant/agently-core/service/workspace"
	"github.com/viant/agently/metadata"
	"github.com/viant/forge/backend/types"
	"gopkg.in/yaml.v3"
)

func TestEmbeddedChatMetaDatasourcePreservesDeclaredHTTPDelivery(t *testing.T) {
	for _, key := range []string{"AGENTLY_DB_DRIVER", "AGENTLY_DB_DSN", "AGENTLY_DB_PATH", "AGENTLY_DB_SECRETS"} {
		t.Setenv(key, "")
	}
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "extension/forge"), 0700))
	entries, err := hostruntime.DefaultFrameworkWindows(&metadata.FS)
	require.NoError(t, err)
	rt, _, _, err := appserver.BuildWorkspaceRuntime(context.Background(), appserver.RuntimeOptions{WorkspaceRoot: root, Defaults: &config.Defaults{}, SkipRegistryInitialize: true, ConfigureBuilder: func(_ context.Context, b *executor.Builder) error { b.WithNativeWindowDefaults(entries); return nil }})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, rt.Close(context.Background())) })
	windowHandler := uihttp.NewEmbeddedHandlerWithAuthorization("embed://localhost/window", nil, rt.AuthorizationPolicy, nil, uihttp.WithWindowResourceProvider(rt.UIBridge))
	host := http.NewServeMux()
	svcworkspace.NewMetadataHandler(rt.Defaults, rt.Store, "native-http-test").Register(host)
	for _, target := range []struct{ query, file string }{{"", "window/chat/new/datasource/meta.yaml"}, {"?platform=web&formFactor=desktop&surface=browser", "window/chat/new/web/datasource/meta.yaml"}, {"?platform=ios&formFactor=phone", "window/chat/new/ios/phone/datasource/meta.yaml"}, {"?platform=android&formFactor=phone", "window/chat/new/android/phone/datasource/meta.yaml"}} {
		t.Run(target.file, func(t *testing.T) {
			raw, err := metadata.FS.ReadFile(target.file)
			require.NoError(t, err)
			var original types.DataSource
			require.NoError(t, yaml.Unmarshal(raw, &original))
			response := httptest.NewRecorder()
			windowHandler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/window/"+url.PathEscape("chat/new")+target.query, nil))
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			var envelope struct {
				Data *types.Window `json:"data"`
			}
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
			require.NotNil(t, envelope.Data)
			delivered, ok := envelope.Data.DataSource["meta"]
			require.True(t, ok)
			require.Equal(t, original.Service, delivered.Service, "native HTTP service has no Core backend and must retain its declared route")
			require.Equal(t, original.Selectors, delivered.Selectors, "metadata response uses its original empty data selector")
			require.Equal(t, original.On, delivered.On, "original chat metadata fetch hooks remain intact")
			require.Equal(t, "agentlyAPI", delivered.Service.Endpoint)
			metaResponse := httptest.NewRecorder()
			host.ServeHTTP(metaResponse, httptest.NewRequest(delivered.Service.Method, delivered.Service.URI, nil))
			require.Equal(t, http.StatusOK, metaResponse.Code, metaResponse.Body.String())
			var payload svcworkspace.MetadataResponse
			require.NoError(t, json.Unmarshal(metaResponse.Body.Bytes(), &payload))
			require.Equal(t, "native-http-test", payload.Version)
		})
	}
}
