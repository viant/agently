package agently

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corecfg "github.com/viant/agently-core/protocol/mcp/config"
	"github.com/viant/agently-core/protocol/mcp/manager"
	identity "github.com/viant/agently-core/protocol/resource"
	"github.com/viant/agently-core/service/policy"
	resources "github.com/viant/agently-core/service/resource"
	windowloader "github.com/viant/agently-core/service/ui/window"
	"github.com/viant/authz"
	"github.com/viant/authz/gating"
	mcp "github.com/viant/mcp"
	mcpclient "github.com/viant/mcp/client"
)

func TestNativeTemplateDoesNotRequireNewResourceACLAndRetainsDeclaredRoles(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(root, "catalog.yaml")
	require.NoError(t, os.WriteFile(path, []byte("windows:\n  - windowId: baseline\n  - windowId: restricted\n    roles: [operator]\n"), 0600))
	config := &hostAuthorizationFile{nativeWindows: true, CatalogPath: path, WindowResources: []windowloader.ResourceBinding{{WindowKey: "baseline", URI: "window://steward/baseline"}, {WindowKey: "restricted", URI: "window://steward/restricted"}}}
	principal := gating.Principal{Facts: authz.Facts{Subject: "alice", Issuer: "verified", Tenant: "tenant", ValidUntil: time.Now().Add(time.Minute)}, AccountID: "account", IdentityRevision: "credential-revision"}
	profileCalls := 0
	profile := &reportHostIdentity{principal}
	profile.principal.IdentityRevision = "profile-revision"
	visible, err := newInternalWindowVisibilityLease(config, principalResolverFunc(func(ctx context.Context) (gating.Principal, error) {
		profileCalls++
		return profile.ResolvePrincipal(ctx)
	}))
	require.NoError(t, err)
	native := &nativeWindowAuthority{principal: func(context.Context) (gating.Principal, error) { return principal, nil }, visibilityLease: visible, allowed: map[string]resources.LocalResourceBinding{}}
	for _, b := range config.WindowResources {
		native.allowed[b.URI] = resources.LocalResourceBinding{URI: b.URI}
	}
	candidate := identity.ResourceCandidate{Kind: identity.WorkingCandidate, ContentFingerprint: identity.ContentFingerprint(json.RawMessage(`{"native":true}`))}
	p := &nativeWindowPolicy{authority: native}
	decision, err := p.SelectRevision(ctx, identity.ResourceRef{URI: "window://steward/baseline"}, []identity.ResourceCandidate{candidate})
	require.NoError(t, err)
	require.NotEmpty(t, decision.AuthorityBinding)
	require.Zero(t, profileCalls)
	_, err = p.SelectRevision(ctx, identity.ResourceRef{URI: "window://steward/restricted"}, []identity.ResourceCandidate{candidate})
	require.ErrorIs(t, err, identity.ErrResourceDenied)
	require.Equal(t, 1, profileCalls)
	profile.principal.Facts.Roles = []string{"operator"}
	profile.principal.Facts.ValidUntil = time.Now().Add(5 * time.Second)
	restrictedDecision, err := p.SelectRevision(ctx, identity.ResourceRef{URI: "window://steward/restricted"}, []identity.ResourceCandidate{candidate})
	require.NoError(t, err)
	require.False(t, restrictedDecision.ValidUntil.After(profile.principal.Facts.ValidUntil), "role profile lease must bound delivered definition")
	_, err = p.SelectRevision(ctx, identity.ResourceRef{URI: "window://steward/baseline", Revision: "2"}, []identity.ResourceCandidate{candidate})
	require.ErrorIs(t, err, identity.ErrResourceDenied)
	_, err = p.SelectRevision(ctx, identity.ResourceRef{URI: "window://users/baseline"}, []identity.ResourceCandidate{candidate})
	require.ErrorIs(t, err, identity.ErrResourceDenied)
	// Scope affects identity only inside trusted native metadata calls and ends
	// before the caller executes protected report/data/component operations.
	fallbackCalls := 0
	actor := native.resolveActor(func(context.Context) (identity.VerifiedActor, error) {
		fallbackCalls++
		return identity.VerifiedActor{}, policy.ErrIdentityRejected
	})
	_, err = actor(ctx)
	require.ErrorIs(t, err, policy.ErrIdentityRejected)
	read, closeRead := native.begin(ctx)
	_, err = actor(read)
	require.NoError(t, err)
	require.Equal(t, 1, fallbackCalls)
	closeRead()
	_, err = actor(read)
	require.ErrorIs(t, err, policy.ErrIdentityRejected)
	require.Equal(t, 2, fallbackCalls)
}

type principalResolverFunc func(context.Context) (gating.Principal, error)

func (f principalResolverFunc) ResolvePrincipal(ctx context.Context) (gating.Principal, error) {
	return f(ctx)
}

func TestNativeTemplateScopeSurvivesActualLocalMCPListAndGet(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "extension/forge/windows"), 0700))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "extension/forge/reporting"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "extension/forge/windows/baseline.yaml"), []byte("view:\n content: {id: baseline}\n"), 0600))
	config := &hostAuthorizationFile{WindowResources: []windowloader.ResourceBinding{{WindowKey: "baseline", URI: "window://steward/baseline"}}}
	snapshot, err := newInternalWindowSnapshot(ctx, root, "", config)
	require.NoError(t, err)
	principal := gating.Principal{Facts: authz.Facts{Subject: "alice", Issuer: "verified", Tenant: "tenant", ValidUntil: time.Now().Add(time.Minute)}, AccountID: "account", IdentityRevision: "credential-revision"}
	native := &nativeWindowAuthority{principal: func(context.Context) (gating.Principal, error) { return principal, nil }, allowed: map[string]resources.LocalResourceBinding{"window://steward/baseline": {URI: "window://steward/baseline"}}}
	actor := native.resolveActor(func(context.Context) (identity.VerifiedActor, error) {
		return identity.VerifiedActor{}, policy.ErrIdentityRejected
	})
	binding := native.allowed["window://steward/baseline"]
	binding.FormatVersion = 2
	binding.Resolver = func(context.Context, identity.VerifiedActor, string) (*identity.ResourceResolver, error) {
		return &identity.ResourceResolver{ProviderIdentity: "internal", Source: snapshot, Policy: &nativeWindowPolicy{authority: native}}, nil
	}
	authorize, err := internalNamespaceIsolation(config, []resources.LocalResourceBinding{binding}, native)
	require.NoError(t, err)
	local, err := resources.NewLocalProvider(resources.LocalConfig{ProviderIdentity: "internal", Actor: actor, Verify: verifyActor(actor), Authorize: authorize, Bindings: []resources.LocalResourceBinding{binding}, WindowIndex: snapshot.WindowIndex, Validators: map[string]resources.LocalResourceValidator{"window": resources.ValidateWindowBundle}})
	require.NoError(t, err)
	mgr, err := manager.New(nil)
	require.NoError(t, err)
	require.NoError(t, mgr.RegisterLocal(ctx, "internal", &corecfg.MCPClient{ClientOptions: &mcp.ClientOptions{}, PrimitiveProviderIdentity: "internal"}, func(context.Context) (mcpclient.Interface, error) { return resources.NewLocalMCPClient(local) }))
	gateway := resources.NewGateway(mgr, actor, verifyActor(actor), "gateway")
	defer gateway.Close()
	read, closeRead := native.begin(ctx)
	defer closeRead()
	rows, err := gateway.List(read, "window", "")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	got, err := gateway.Get(read, rows[0].Connection, identity.ResourceRef{URI: binding.URI}, nil)
	require.NoError(t, err)
	require.Equal(t, binding.URI, got.ResolvedResource.URI)
}

func TestNativeSnapshotCapturesAfterStartupAndRejectsLaterSourceChanges(t *testing.T) {
	for _, change := range []string{"content", "import", "mode"} {
		t.Run(change, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			dir := filepath.Join(root, "extension/forge/windows/shared")
			require.NoError(t, os.MkdirAll(dir, 0700))
			require.NoError(t, os.MkdirAll(filepath.Join(root, "extension/forge/reporting"), 0700))
			main := filepath.Join(root, "extension/forge/windows/baseline.yaml")
			imported := filepath.Join(dir, "card.yaml")
			require.NoError(t, os.WriteFile(main, []byte("$import(shared/card.yaml)\n"), 0644))
			require.NoError(t, os.WriteFile(imported, []byte("view:\n content: {id: baseline}\n"), 0644))
			config := &hostAuthorizationFile{WindowResources: []windowloader.ResourceBinding{{WindowKey: "baseline", URI: "window://steward/baseline"}}}
			snapshot, err := newInternalWindowSnapshot(ctx, root, "", config, true)
			require.NoError(t, err)
			require.Zero(t, snapshot.CompileCount())
			// Model constructor directory permission normalization before source
			// publication. The compatibility fix itself changes no filesystem modes.
			require.NoError(t, os.Chmod(dir, 0755))
			require.NoError(t, snapshot.Initialize(ctx))
			require.EqualValues(t, 1, snapshot.CompileCount())
			rows, err := snapshot.WindowIndex(ctx)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			uri, _ := identity.ParseResourceURI(config.WindowResources[0].URI)
			candidates, err := snapshot.Candidates(ctx, uri)
			require.NoError(t, err)
			require.Len(t, candidates, 1)
			switch change {
			case "content":
				require.NoError(t, os.WriteFile(main, []byte("view:\n content: {id: replaced}\n"), 0644))
			case "import":
				require.NoError(t, os.WriteFile(imported, []byte("view:\n content: {id: replaced}\n"), 0644))
			case "mode":
				require.NoError(t, os.Chmod(imported, 0600))
			}
			require.ErrorIs(t, snapshot.CheckCandidate(ctx, uri, candidates[0]), identity.ErrResourceStale)
			_, err = snapshot.WindowIndex(ctx)
			require.ErrorIs(t, err, identity.ErrResourceStale)
			require.EqualValues(t, 1, snapshot.CompileCount(), "serving cannot rebuild a changed immutable generation")
		})
	}
}
