package agently

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/viant/agently-core/app/executor"
	forgetypes "github.com/viant/forge/backend/types"
	"gopkg.in/yaml.v3"
)

func TestHostAuthorizationHeadlessReportProof(t *testing.T) {
	for _, supplied := range []bool{false, true} {
		t.Run(map[bool]string{false: "generated", true: "existing"}[supplied], func(t *testing.T) {
			root := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(root, "catalog.yaml"), []byte("baseURL: .\nwindows: []\n"), 0600))
			cfg := hostAuthorizationFile{SchemaVersion: 1, ProviderRef: "verified", CapabilityMappingRef: "mapped", PolicyVersion: "one", CatalogPath: "catalog.yaml", Identity: json.RawMessage(`{"binding":"host-owned"}`)}
			data, err := json.Marshal(cfg)
			require.NoError(t, err)
			path := filepath.Join(root, "authz.json")
			require.NoError(t, os.WriteFile(path, data, 0600))
			ws := authorizationWorkspaceConfig("authz", "verified", "mapped")
			var node yaml.Node
			require.NoError(t, yaml.Unmarshal([]byte("reporting:\n  enabled: true\n"), &node))
			ws.DefaultNode = *node.Content[0]
			opts := ServeOptions{AuthzConfigPath: path, AuthorizationProviderFactory: func(context.Context, json.RawMessage, json.RawMessage) (AuthorizationProviders, error) {
				return AuthorizationProviders{Identity: fixtureHostIdentity{}, AllowsTenant: func(string) bool { return true }}, nil
			}}
			if supplied {
				opts.WindowTargetProof, err = forgetypes.NewWindowTargetHMAC(make([]byte, 32))
				require.NoError(t, err)
			}
			configure, err := configureHostAuthorization(opts, root, ws, nil)
			require.NoError(t, err)
			builder := executor.NewBuilder()
			require.NoError(t, configure(context.Background(), builder))
			value := reflect.ValueOf(builder).Elem()
			require.False(t, value.FieldByName("primitiveReportDependencyProof").IsNil())
			require.False(t, value.FieldByName("primitiveActor").IsNil())
			require.False(t, value.FieldByName("primitiveVerifier").IsNil())
			require.True(t, value.FieldByName("primitiveWindowAdmission").IsNil(), "report proof must not enable delegated windows")
			require.True(t, value.FieldByName("primitiveTargetProof").IsNil(), "report proof must not enter window pair")
			if supplied {
				require.Equal(t, reflect.ValueOf(opts.WindowTargetProof).Pointer(), value.FieldByName("primitiveReportDependencyProof").Elem().Pointer())
			}
		})
	}
}
