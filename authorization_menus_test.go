package agently

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"github.com/viant/agently-core/app/executor"
	corecfg "github.com/viant/agently-core/protocol/mcp/config"
	"github.com/viant/agently-core/protocol/mcp/manager"
	identity "github.com/viant/agently-core/protocol/resource"
	"github.com/viant/agently-core/service/policy"
	resources "github.com/viant/agently-core/service/resource"
	windowloader "github.com/viant/agently-core/service/ui/window"
	"github.com/viant/authz"
	"github.com/viant/authz/gating"
	"github.com/viant/authz/oauth"
	mcp "github.com/viant/mcp"
	mcpclient "github.com/viant/mcp/client"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHostMenuConfigurationIsExplicitAndConfinedToTrustedPaths(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "authorization.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"schemaVersion":1,"catalogPath":"catalog.yaml","menuSources":[{"uri":"menu://steward/navigation","localLayoutPath":"layout.yaml"}]}`), 0600))
	config, err := readHostAuthorization(path)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "layout.yaml"), config.MenuSources[0].LocalLayoutPath)
	for _, source := range []HostMenuSource{{URI: "menu://platform/navigation"}, {URI: "https://example.com/navigation", ProviderIdentity: "external"}, {URI: "menu://users/navigation", LocalLayoutPath: "layout.yaml"}, {URI: "menu://platform/navigation", ProviderIdentity: "external", Revision: "../escape"}} {
		require.Error(t, validateHostMenuSources(&hostAuthorizationFile{MenuSources: []HostMenuSource{source}}, path))
	}
	require.Nil(t, configureLayoutMenuReader(nil, "", nil))
	_, err = configuredMenuBindings("internal", &hostAuthorizationFile{MenuSources: []HostMenuSource{{URI: "menu://platform/navigation", LocalLayoutPath: filepath.Join(root, "layout.yaml"), ProviderIdentity: "foreign"}}}, nil)
	require.Error(t, err)
}

func TestHostMenuVisibilityUsesOnlyDeclaredCatalogAndNamespace(t *testing.T) {
	config := &hostAuthorizationFile{CatalogPath: filepath.Join(t.TempDir(), "catalog.yaml"), WindowResources: []windowloader.ResourceBinding{{URI: "window://steward/plain", WindowKey: "plain"}, {URI: "window://steward/guarded", WindowKey: "guarded"}}}
	require.NoError(t, os.WriteFile(config.CatalogPath, []byte("windows:\n  - windowId: guarded\n    roles: [admin]\n"), 0600))
	lease := time.Now().Add(time.Minute)
	principal := &visibilityPrincipal{principal: gating.Principal{Facts: authz.Facts{Subject: "alice", Issuer: "issuer", Tenant: "tenant", Roles: []string{"reader"}, ValidUntil: lease}, AccountID: "account", IdentityRevision: "one"}}
	actor := identity.VerifiedActor{Subject: "alice", Issuer: "issuer", TenantID: "tenant", AccountID: "account", IdentityRevision: "one", ValidUntil: lease}
	calls := 0
	visible, err := configuredMenuVisibility(config, principal, func(_ context.Context, _ identity.VerifiedActor, uri identity.ResourceURI, _ string) error {
		calls++
		require.Equal(t, "window", uri.Kind)
		return nil
	})
	require.NoError(t, err)
	menuURI, _ := identity.ParseResourceURI("menu://steward/navigation")
	ok, err := visible(context.Background(), actor, menuURI, json.RawMessage(`{"type":"window","windowKey":"plain","parameters":{"foreignBusinessId":7}}`))
	require.NoError(t, err)
	require.True(t, ok)
	require.Zero(t, principal.calls, "plain metadata cannot evaluate invented business permission")
	ok, err = visible(context.Background(), actor, menuURI, json.RawMessage(`{"type":"window","windowKey":"guarded"}`))
	require.NoError(t, err)
	require.False(t, ok)
	before := calls
	ok, err = visible(context.Background(), actor, menuURI, json.RawMessage(`{"type":"window","windowKey":"notDeclared"}`))
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, before, calls)
	principal.principal.Facts.Roles = []string{"admin"}
	ok, err = visible(context.Background(), actor, menuURI, json.RawMessage(`{"type":"window","windowKey":"guarded"}`))
	require.NoError(t, err)
	require.True(t, ok)
	principal.principal.AccountID = "foreign"
	_, err = visible(context.Background(), actor, menuURI, json.RawMessage(`{"type":"window","windowKey":"guarded"}`))
	require.Error(t, err)
}

func TestHostConfiguredMenuMCPReaderPreservesActionsAndRevalidates(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	layoutPath := filepath.Join(root, "layout.yaml")
	uri := "menu://steward/navigation"
	raw := []byte("version: 1\nid: steward\ntopbar: {actions: [{id: automation, title: Automation, action: {type: window, windowKey: schedule}}]}\napplications:\n  - id: steward\n    title: Workspace\n    icon: applications\n    menus:\n      - id: advertisers\n        title: Advertisers\n        icon: office\n        action: {type: window, windowKey: advertiserList, parameters: {selected: 9}}\n      - id: campaigns\n        title: Campaigns\n        icon: timeline-events\n        action: {type: window, windowKey: campaignList}\n      - id: denied\n        title: Denied group\n        children:\n          - id: secret\n            title: Secret\n            action: {type: window, windowKey: secretWindow}\n")
	require.NoError(t, os.WriteFile(layoutPath, raw, 0600))
	catalogPath := filepath.Join(root, "catalog.yaml")
	require.NoError(t, os.WriteFile(catalogPath, []byte("windows:\n  - windowId: secretWindow\n    roles: [admin]\n"), 0600))
	config := &hostAuthorizationFile{CatalogPath: catalogPath, MenuSources: []HostMenuSource{{URI: uri, LocalLayoutPath: layoutPath}}}
	lease := time.Now().Add(time.Minute)
	principal := &reportHostIdentity{gating.Principal{Facts: authz.Facts{Subject: "alice", Issuer: "issuer", Tenant: "tenant", Roles: []string{"reader"}, ValidUntil: lease}, AccountID: "account", IdentityRevision: "one"}}
	resource := authz.Resource{Kind: "menu", ID: uri, Tenant: "tenant", Version: "working"}
	family := authz.ResourceFamily{Kind: "menu", ID: uri, Tenant: "tenant"}
	config.Policies = []authz.Document{{Resource: resource, Revision: 1, Policies: map[string]authz.Policy{"read": {Mode: "protected", Rule: &authz.Rule{Kind: "role", Value: "reader"}}}}}
	config.Requirements = []gating.Binding{{Resource: resource, Action: "read", Document: gating.RequirementsDocument{Revision: "one", Requirements: gating.Requirements{SchemaVersion: 1}}}}
	config.ResourceRevisionBindings = []policy.ResourceRevisionBinding{{Operation: hostMenuOperation, URI: uri, Resource: family, Action: "read"}}
	for _, key := range []string{"advertiserList", "campaignList", "secretWindow"} {
		windowURI := "window://steward/" + key
		config.WindowResources = append(config.WindowResources, windowloader.ResourceBinding{WindowKey: key, URI: windowURI})
		config.ResourceRevisionBindings = append(config.ResourceRevisionBindings, policy.ResourceRevisionBinding{Operation: policy.OperationWindowView, URI: windowURI, Resource: authz.ResourceFamily{Kind: "window", ID: windowURI, Tenant: "tenant"}, Action: "read"})
	}
	bundle, err := oauth.NewStaticAuthorization(oauth.StaticAuthorizationConfig{Identity: principal, AllowsTenant: func(s string) bool { return s == "tenant" }, Policies: config.Policies, Requirements: config.Requirements})
	require.NoError(t, err)
	mappings, err := authz.NewStaticSelectionStore([]authz.SelectionDocument{{Resource: family, Revision: 1, DefaultVersion: "working"}})
	require.NoError(t, err)
	prepared, err := executor.PrepareStaticAuthorization(executor.StaticAuthorizationRegistration{ProviderRef: "identity", CapabilityMappingRef: "mapping", PolicyVersion: "one", Service: bundle.ACL, Account: principal.Account, AuthorityRevision: principal.AuthorityRevision, AuthoritySnapshot: principal.ResolvePrincipal, GateEvaluator: bundle.Gates, ResourceRevisionBindings: config.ResourceRevisionBindings, ResourceRevisionMappings: mappings})
	require.NoError(t, err)
	sharedSnapshot, err := newInternalWindowSnapshot(ctx, root, "", &hostAuthorizationFile{MenuSources: config.MenuSources})
	require.NoError(t, err)
	bindings, err := configuredMenuBindings("internal", config, prepared, sharedSnapshot)
	require.NoError(t, err)
	require.Len(t, bindings, 1)
	heldActor, err := prepared.ResourceActor(ctx)
	require.NoError(t, err)
	sharedResolver, err := bindings[0].Resolver(ctx, heldActor, "resource.get")
	require.NoError(t, err)
	require.Same(t, sharedSnapshot, sharedResolver.Source, "same-root menus share the existing workspace native snapshot")
	namespace, err := internalNamespaceIsolation(config, append(append([]resources.LocalResourceBinding(nil), bindings...), resources.LocalResourceBinding{URI: "window://steward/advertiserList"}, resources.LocalResourceBinding{URI: "window://steward/campaignList"}, resources.LocalResourceBinding{URI: "window://steward/secretWindow"}))
	require.NoError(t, err)
	visible, err := configuredMenuVisibility(config, principal, namespace)
	require.NoError(t, err)
	actor := resources.ActorResolver(prepared.ResourceActor)
	provider, err := resources.NewLocalProvider(resources.LocalConfig{ProviderIdentity: "internal", Actor: actor, Verify: verifyActor(actor), Authorize: namespace, Bindings: bindings, Validators: map[string]resources.LocalResourceValidator{"menu": resources.ValidateMenu}, MenuActionVisibility: visible})
	require.NoError(t, err)
	mgr, err := manager.New(nil)
	require.NoError(t, err)
	defer mgr.CloseConversation("")
	require.NoError(t, mgr.RegisterLocal(ctx, "explicit-owner", &corecfg.MCPClient{ClientOptions: &mcp.ClientOptions{}, PrimitiveProviderIdentity: "internal"}, func(context.Context) (mcpclient.Interface, error) { return resources.NewLocalMCPClient(provider) }))
	gateway := resources.NewGateway(mgr, actor, verifyActor(actor), "consumer")
	defer gateway.Close()
	runtime := &executor.Runtime{PrimitiveProviders: gateway}
	factory := configureLayoutMenuReader(config.MenuSources, "internal", prepared)
	reader, err := factory(runtime)
	require.NoError(t, err)
	got, err := reader(ctx)
	require.NoError(t, err)
	text := string(got.Applications)
	require.Contains(t, text, "advertiserList")
	require.Contains(t, text, "campaignList")
	require.Contains(t, text, `"parameters":{"selected":9}`)
	require.NotContains(t, text, "secretWindow")
	require.NotContains(t, text, "Denied group")
	require.NotContains(t, text, "automation")
	require.NoError(t, got.Revalidate(ctx))
	external, err := configureLayoutMenuReader([]HostMenuSource{{URI: uri, ProviderIdentity: "internal"}}, "", prepared)(runtime)
	require.NoError(t, err)
	externalRead, err := external(ctx)
	require.NoError(t, err, "explicit external owner is supported without transport URL inference")
	require.NoError(t, externalRead.Revalidate(ctx))
	got, err = reader(ctx)
	require.NoError(t, err)
	principal.principal.AccountID = "foreign"
	require.Error(t, got.Revalidate(ctx))
	_ = got.Close()
	principal.principal.AccountID = "account"
	got, err = reader(ctx)
	require.NoError(t, err)
	principal.principal.Facts.Roles = nil
	_, err = reader(ctx)
	require.Error(t, err)
	principal.principal.Facts.Roles = []string{"reader"}
	principal.principal.Facts.ValidUntil = time.Now().Add(-time.Second)
	_, err = reader(ctx)
	require.Error(t, err)
	principal.principal.Facts.ValidUntil = lease
	got, err = reader(ctx)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(layoutPath, append(raw, []byte("# unchanged authored content\n")...), 0600))
	require.Error(t, got.Revalidate(ctx), "startup source generation changes require explicit restart")
	_ = got.Close()
	_, err = reader(ctx)
	require.Error(t, err, "changed source cannot hot reload through a warm reader")
	// Explicit restart compiles the new authored generation; original actors,
	// policies and action visibility remain unchanged.
	refreshed, err := configuredMenuBindings("internal", config, prepared)
	require.NoError(t, err)
	restarted, err := resources.NewLocalProvider(resources.LocalConfig{ProviderIdentity: "internal", Actor: actor, Verify: verifyActor(actor), Authorize: namespace, Bindings: refreshed, Validators: map[string]resources.LocalResourceValidator{"menu": resources.ValidateMenu}, MenuActionVisibility: visible})
	require.NoError(t, err)
	freshManager, err := manager.New(nil)
	require.NoError(t, err)
	defer freshManager.CloseConversation("")
	require.NoError(t, freshManager.RegisterLocal(ctx, "explicit-owner", &corecfg.MCPClient{ClientOptions: &mcp.ClientOptions{}, PrimitiveProviderIdentity: "internal"}, func(context.Context) (mcpclient.Interface, error) { return resources.NewLocalMCPClient(restarted) }))
	freshGateway := resources.NewGateway(freshManager, actor, verifyActor(actor), "consumer")
	defer freshGateway.Close()
	freshReader, err := factory(&executor.Runtime{PrimitiveProviders: freshGateway})
	require.NoError(t, err)
	got, err = freshReader(ctx)
	require.NoError(t, err)
	require.NoError(t, got.Revalidate(ctx))
	got, err = freshReader(ctx)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(layoutPath, []byte("version: 1\nid: steward\napplications: []\n"), 0600))
	require.Error(t, got.Revalidate(ctx))
	_ = got.Close()

}
