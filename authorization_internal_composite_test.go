package agently

import (
	"context"
	"github.com/stretchr/testify/require"
	"github.com/viant/agently-core/app/executor"
	identity "github.com/viant/agently-core/protocol/resource"
	forge "github.com/viant/agently-core/service/primitiveprovider"
	resources "github.com/viant/agently-core/service/resource"
	"github.com/viant/forge/backend/types"
	"strings"
	"testing"
)

func TestLateInternalCatalogPreservesDelegatedCatalogAdmissionAndProof(t *testing.T) {
	for _, delegated := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy-local", true: "delegated-and-local"}[delegated], func(t *testing.T) {
			oldLocal := &internalWindowCatalog{}
			local := &internalWindowCatalog{}
			bridge := forge.NewService(&forge.Config{WindowDefinitions: oldLocal})
			runtime := &executor.Runtime{UIBridge: bridge}
			var remote *resources.WindowCatalog
			if delegated {
				proof, err := types.NewWindowTargetHMAC([]byte(strings.Repeat("p", 32)))
				require.NoError(t, err)
				remote = &resources.WindowCatalog{Gateway: &resources.Gateway{}, TargetProof: proof, Admission: func(context.Context, identity.ResolvedResource, *types.Window) error {
					return identity.ErrResourceDenied
				}}
				runtime.PrimitiveWindows = &resources.CompositeWindowCatalog{Local: oldLocal, LocalProviderIdentity: "old", Remote: remote}
			}
			bindInternalWindowCatalog(runtime, "internal", local)
			bridge.ConfigureWindowCatalog(func(actual forge.WindowDefinitionCatalog) forge.WindowDefinitionCatalog {
				if !delegated {
					require.Same(t, local, actual)
					return actual
				}
				composite, ok := actual.(*resources.CompositeWindowCatalog)
				require.True(t, ok)
				require.Same(t, runtime.PrimitiveWindows, composite)
				require.Same(t, local, composite.Local)
				require.Equal(t, "internal", composite.LocalProviderIdentity)
				require.Same(t, remote, composite.Remote)
				require.NotNil(t, composite.Remote.Gateway)
				require.NotNil(t, composite.Remote.TargetProof)
				require.ErrorIs(t, composite.Remote.Admission(context.Background(), identity.ResolvedResource{}, nil), identity.ErrResourceDenied)
				return actual
			})
		})
	}
}
