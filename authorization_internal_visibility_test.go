package agently

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	identity "github.com/viant/agently-core/protocol/resource"
	"github.com/viant/agently-core/service/policy"
	resources "github.com/viant/agently-core/service/resource"
	windowloader "github.com/viant/agently-core/service/ui/window"
	"github.com/viant/authz"
	"github.com/viant/authz/gating"
)

type visibilityPrincipal struct {
	principal gating.Principal
	calls     int
}

func (p *visibilityPrincipal) ResolvePrincipal(context.Context) (gating.Principal, error) {
	p.calls++
	return p.principal, nil
}

func TestInternalWindowVisibilityChecksOnlyDeclaredFeaturesAndRoles(t *testing.T) {
	ctx := context.Background()
	uri := "window://platform/grouped"
	config := &hostAuthorizationFile{CatalogPath: filepath.Join(t.TempDir(), "catalog.yaml"), WindowResources: []windowloader.ResourceBinding{{URI: uri, WindowKey: "grouped"}, {URI: "window://platform/plain", WindowKey: "plain"}}, ResourceRevisionBindings: []policy.ResourceRevisionBinding{{URI: uri, Operation: policy.OperationWindowView, Resource: authz.ResourceFamily{Kind: "window", ID: uri, Tenant: "tenant"}, Action: "read"}}, Requirements: []gating.Binding{{Resource: authz.Resource{Kind: "window", ID: uri, Tenant: "tenant", Version: "working"}, Action: "read", Document: gating.RequirementsDocument{Requirements: gating.Requirements{SchemaVersion: 1, RequiredExposures: []string{"feature"}, AllowedRoles: []string{"reader"}, Entity: &gating.EntityRequirement{Type: "order", Permission: "read", SelectionMode: "single"}}}}}}
	require.NoError(t, os.WriteFile(config.CatalogPath, []byte("windows: []\n"), 0600))
	lease := time.Now().Add(time.Minute)
	principal := &visibilityPrincipal{principal: gating.Principal{Facts: authz.Facts{Subject: "alice", Issuer: "issuer", Tenant: "tenant", Roles: []string{"reader"}, Exposures: []string{"feature"}, ValidUntil: lease}, AccountID: "account", IdentityRevision: "one"}}
	actor := identity.VerifiedActor{Subject: "alice", Issuer: "issuer", TenantID: "tenant", AccountID: "account", IdentityRevision: "one", ValidUntil: lease}
	visible, err := internalWindowVisibility(config, principal)
	require.NoError(t, err)
	for i := 0; i < 20; i++ {
		ok, err := visible(ctx, actor, resources.LocalResourceBinding{URI: "window://platform/plain"})
		require.NoError(t, err)
		require.True(t, ok)
	}
	require.Zero(t, principal.calls, "roleless/featureless metadata must not evaluate permission")
	ok, err := visible(ctx, actor, resources.LocalResourceBinding{URI: uri})
	require.NoError(t, err)
	require.True(t, ok, "entity requirement must not be evaluated by metadata discovery")
	principal.principal.Facts.Exposures = nil
	ok, err = visible(ctx, actor, resources.LocalResourceBinding{URI: uri})
	require.NoError(t, err)
	require.False(t, ok)
	principal.principal.Facts.Exposures = []string{"feature"}
	principal.principal.Facts.Roles = nil
	ok, err = visible(ctx, actor, resources.LocalResourceBinding{URI: uri})
	require.NoError(t, err)
	require.False(t, ok)
	principal.principal.AccountID = "foreign"
	_, err = visible(ctx, actor, resources.LocalResourceBinding{URI: uri})
	require.Error(t, err)
}

func TestInternalNamespaceIsolationDoesNotGrantPrivateOrForeignMembership(t *testing.T) {
	config := &hostAuthorizationFile{ResourceRevisionBindings: []policy.ResourceRevisionBinding{{URI: "window://platform/orders", Resource: authz.ResourceFamily{Kind: "window", ID: "window://platform/orders", Tenant: "tenant"}}, {URI: "window://users/alice/orders", Resource: authz.ResourceFamily{Kind: "window", ID: "window://users/alice/orders", Tenant: "tenant"}}}}
	isolate, err := internalNamespaceIsolation(config, []resources.LocalResourceBinding{{URI: "window://platform/orders"}, {URI: "window://users/alice/orders"}})
	require.NoError(t, err)
	actor := identity.VerifiedActor{Subject: "alice", Issuer: "issuer", TenantID: "tenant", AccountID: "account", IdentityRevision: "one", ValidUntil: time.Now().Add(time.Minute)}
	uri, _ := identity.ParseResourceURI("window://platform/orders")
	require.NoError(t, isolate(context.Background(), actor, uri, "resource.list"))
	private, _ := identity.ParseResourceURI("window://users/alice/orders")
	require.ErrorIs(t, isolate(context.Background(), actor, private, "resource.list"), identity.ErrResourceDenied)
	actor.TenantID = "foreign"
	require.ErrorIs(t, isolate(context.Background(), actor, uri, "resource.list"), identity.ErrResourceDenied)
}
