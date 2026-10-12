package agently

import (
	"context"
	"github.com/stretchr/testify/require"
	"github.com/viant/agently-core/app/executor"
	"github.com/viant/agently-core/app/executor/config"
	appserver "github.com/viant/agently-core/app/server"
	forge "github.com/viant/agently-core/service/primitiveprovider"
	"os"
	"path/filepath"
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
	require.Equal(t, "window://workspace/chat/new", got.Definition.Resource.URI)
	require.NotNil(t, got.Definition.View.Content)
}
