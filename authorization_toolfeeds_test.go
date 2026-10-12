package agently

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/viant/agently-core/app/executor"
	execconfig "github.com/viant/agently-core/app/executor/config"
	appserver "github.com/viant/agently-core/app/server"
	corecfg "github.com/viant/agently-core/protocol/mcp/config"
	"github.com/viant/agently-core/protocol/mcp/manager"
	primitive "github.com/viant/agently-core/protocol/primitive"
	identity "github.com/viant/agently-core/protocol/resource"
	"github.com/viant/agently-core/sdk"
	api "github.com/viant/agently-core/sdk/api"
	svcauth "github.com/viant/agently-core/service/auth"
	"github.com/viant/agently-core/service/policy"
	resources "github.com/viant/agently-core/service/resource"
	"github.com/viant/agently-core/workspace/codec"
	"github.com/viant/authz"
	"github.com/viant/authz/gating"
	"github.com/viant/authz/oauth"
	mcp "github.com/viant/mcp"
	mcpclient "github.com/viant/mcp/client"
	scyauth "github.com/viant/scy/auth"
)

func toolfeedHostFixture(t *testing.T) (string, *hostAuthorizationFile, *executor.PreparedAuthorization, *reportHostIdentity) {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "catalog.yaml"), []byte("baseURL: .\nwindows: []\n"), 0600))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "feeds", "nested"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "feeds", "nested", "terminal.yaml"), []byte("id: legacy-terminal\ntitle: Terminal\nmatch:\n  service: system/exec\n  method: execute\n"), 0600))
	uri := "toolfeed://team/nested/terminal"
	family := authz.ResourceFamily{Kind: "toolfeed", ID: uri, Tenant: "tenant"}
	resource := authz.Resource{Kind: "toolfeed", ID: uri, Tenant: "tenant", Version: "working"}
	principal := &reportHostIdentity{gating.Principal{Facts: authz.Facts{Subject: "alice", Issuer: "issuer", Tenant: "tenant", Roles: []string{"reader"}, ValidUntil: time.Now().Add(time.Minute)}, AccountID: "account", IdentityRevision: "one"}}
	config := &hostAuthorizationFile{CatalogPath: filepath.Join(root, "catalog.yaml"), ToolfeedSources: []HostToolfeedSource{{Namespace: "team", Directory: "feeds"}}, Policies: []authz.Document{{Resource: resource, Revision: 1, Policies: map[string]authz.Policy{"read": {Mode: "protected", Rule: &authz.Rule{Kind: "role", Value: "reader"}}}}}, Requirements: []gating.Binding{{Resource: resource, Action: "read", Document: gating.RequirementsDocument{Revision: "one", Requirements: gating.Requirements{SchemaVersion: 1}}}}, ResourceSelections: []authz.SelectionDocument{{Resource: family, Revision: 1, DefaultVersion: "working"}}}
	for _, operation := range []string{"resource.list", "resource.get"} {
		config.ResourceRevisionBindings = append(config.ResourceRevisionBindings, policy.ResourceRevisionBinding{Operation: operation, URI: uri, Resource: family, Action: "read"})
	}
	bundle, err := oauth.NewStaticAuthorization(oauth.StaticAuthorizationConfig{Identity: principal, AllowsTenant: func(v string) bool { return v == "tenant" }, Policies: config.Policies, Requirements: config.Requirements})
	require.NoError(t, err)
	selections, err := authz.NewStaticSelectionStore(config.ResourceSelections)
	require.NoError(t, err)
	prepared, err := executor.PrepareStaticAuthorization(executor.StaticAuthorizationRegistration{ProviderRef: "identity", CapabilityMappingRef: "mapping", PolicyVersion: "one", Service: bundle.ACL, Account: principal.Account, AuthorityRevision: principal.AuthorityRevision, AuthoritySnapshot: principal.ResolvePrincipal, GateEvaluator: bundle.Gates, ResourceRevisionMappings: selections, ResourceRevisionBindings: config.ResourceRevisionBindings})
	require.NoError(t, err)
	return root, config, prepared, principal
}

func TestHostToolfeedsThroughMCPUseOperatorPolicyAndCurrentPins(t *testing.T) {
	ctx := context.Background()
	root, config, prepared, principal := toolfeedHostFixture(t)
	registration := &internalHostRegistration{workspaceRoot: root}
	local, err := configureInternalHostResources("host-resources", config, prepared, nil, nil, nil, nil, nil, nil, principal, registration)
	require.NoError(t, err)
	mgr, err := manager.New(nil)
	require.NoError(t, err)
	require.NoError(t, mgr.RegisterLocal(ctx, "host-resources", &corecfg.MCPClient{ClientOptions: &mcp.ClientOptions{}, PrimitiveProviderIdentity: "host-resources"}, func(context.Context) (mcpclient.Interface, error) { return resources.NewLocalMCPClient(local) }))
	gateway := resources.NewGateway(mgr, prepared.ResourceActor, internalActorVerifier(prepared), "gateway")
	t.Cleanup(gateway.Close)
	rt := &executor.Runtime{PrimitiveProviders: gateway}
	require.NoError(t, registration.Bind(ctx, rt))
	client, err := mgr.Get(ctx, "", "host-resources")
	require.NoError(t, err)
	tools, err := client.ListTools(ctx, nil)
	require.NoError(t, err)
	names := []string{}
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	require.Contains(t, names, "toolfeeds/list")
	require.Contains(t, names, "toolfeeds/get")
	namespaces, err := rt.PrimitiveProviders.Discover(ctx)
	require.NoError(t, err)
	require.Len(t, namespaces, 1)
	require.Equal(t, primitive.ToolfeedSchemaURI, namespaces[0].Kinds[0].SchemaURI)
	rows, err := gateway.List(ctx, "toolfeed", "team")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	uri := "toolfeed://team/nested/terminal"
	require.Equal(t, uri, rows[0].Resource.URI)
	got, err := gateway.Get(ctx, namespaces[0].Connection, identity.ResourceRef{URI: uri}, nil)
	require.NoError(t, err)
	require.Equal(t, "host-resources", got.ResolvedResource.ProviderIdentity)
	require.Equal(t, identity.WorkingCandidate, got.ResolvedResource.Kind)
	var native, projected api.FeedSpec
	file := filepath.Join(root, "feeds", "nested", "terminal.yaml")
	require.NoError(t, codec.DecodeFile(file, &native))
	require.NoError(t, json.Unmarshal(got.Resource.Definition, &projected))
	require.Equal(t, native, projected)
	require.Equal(t, "legacy-terminal", projected.ID)
	for _, ref := range []identity.ResourceRef{{URI: "toolfeed://unknown/nested/terminal"}, {URI: "toolfeed://team/unregistered"}, {URI: uri, Revision: "1"}} {
		_, err = gateway.Get(ctx, namespaces[0].Connection, ref, nil)
		require.Error(t, err)
	}
	held, err := prepared.ResourceActor(ctx)
	require.NoError(t, err)
	principal.principal.IdentityRevision = "two"
	_, err = configuredToolfeedBindings(ctx, root, "host-resources", config.ToolfeedSources, prepared)
	require.NoError(t, err)
	_, err = gateway.Get(ctx, namespaces[0].Connection, identity.ResourceRef{URI: uri}, got.ResolvedResource)
	require.Error(t, err)
	bindings, err := configuredToolfeedBindings(ctx, root, "host-resources", config.ToolfeedSources, prepared)
	require.NoError(t, err)
	_, err = bindings[0].Resolver(ctx, held, "resource.get")
	require.Error(t, err)
	principal.principal.IdentityRevision = "one"
	expired := *got.ResolvedResource
	expired.ValidUntil = time.Now().Add(-time.Second)
	_, err = gateway.Get(ctx, namespaces[0].Connection, identity.ResourceRef{URI: uri}, &expired)
	require.Error(t, err)
	principal.principal.Facts.Roles = nil
	_, err = gateway.Get(ctx, namespaces[0].Connection, identity.ResourceRef{URI: uri}, got.ResolvedResource)
	require.Error(t, err)
	rows, err = gateway.List(ctx, "toolfeed", "team")
	if err == nil {
		require.Empty(t, rows)
	}
	principal.principal.Facts.Roles = []string{"reader"}
	principal.principal.Facts.Tenant = "foreign"
	_, err = gateway.Get(ctx, namespaces[0].Connection, identity.ResourceRef{URI: uri}, nil)
	require.Error(t, err)
	principal.principal.Facts.Tenant = "tenant"
	require.NoError(t, os.WriteFile(file, []byte("id: legacy-terminal\ntitle: Changed\n"), 0600))
	_, err = gateway.Get(ctx, namespaces[0].Connection, identity.ResourceRef{URI: uri}, got.ResolvedResource)
	require.Error(t, err)
	fresh, err := gateway.Get(ctx, namespaces[0].Connection, identity.ResourceRef{URI: uri}, nil)
	require.Error(t, err, "changedstartupassets remain denied until restart")
	require.Nil(t, fresh)
	principal.principal.Facts.ValidUntil = time.Now().Add(-time.Second)
	_, err = gateway.Get(ctx, namespaces[0].Connection, identity.ResourceRef{URI: uri}, nil)
	require.Error(t, err)
}

func TestHostToolfeedSourcesFailClosed(t *testing.T) {
	ctx := context.Background()
	root, config, prepared, principal := toolfeedHostFixture(t)
	for _, source := range []HostToolfeedSource{{Directory: "feeds"}, {Namespace: "team"}, {Namespace: "../team", Directory: "feeds"}, {Namespace: "users", Directory: "feeds"}, {Namespace: "team", Directory: "../feeds"}, {Namespace: "team", Directory: root}, {Namespace: "team", Directory: "missing"}} {
		_, err := configuredToolfeedBindings(ctx, root, "internal", []HostToolfeedSource{source}, prepared)
		require.Error(t, err)
	}
	_, err := configuredToolfeedBindings(ctx, "", "internal", config.ToolfeedSources, prepared)
	require.Error(t, err)
	config.ResourceRevisionBindings = nil
	_, err = configureInternalHostResources("internal", config, prepared, nil, nil, nil, nil, nil, nil, principal, &internalHostRegistration{workspaceRoot: root})
	require.ErrorContains(t, err, "explicit tenant mapping")
	require.NoError(t, os.Symlink(filepath.Join(root, "feeds", "nested", "terminal.yaml"), filepath.Join(root, "feeds", "alias.yaml")))
	_, err = configuredToolfeedBindings(ctx, root, "internal", config.ToolfeedSources, prepared)
	require.Error(t, err)
}

func TestConfiguredHostRuntimeRegistersToolfeedSources(t *testing.T) {
	ctx := svcauth.InjectTokens(context.Background(), &scyauth.Token{IDToken: "trusted-fixture"})
	root, config, _, principal := toolfeedHostFixture(t)
	require.NoError(t, os.Rename(filepath.Join(root, "feeds", "nested", "terminal.yaml"), filepath.Join(root, "feeds", "terminal.yaml")))
	const uri = "toolfeed://team/terminal"
	for i := range config.ResourceRevisionBindings {
		config.ResourceRevisionBindings[i].URI = uri
		config.ResourceRevisionBindings[i].Resource.ID = uri
	}
	config.Policies[0].Resource.ID = uri
	config.Requirements[0].Resource.ID = uri
	config.ResourceSelections[0].Resource.ID = uri
	config.AccountProjection = "opaque"
	config.SchemaVersion = 1
	config.ProviderRef = "identity"
	config.CapabilityMappingRef = "mapping"
	config.PolicyVersion = "one"
	raw, err := json.Marshal(config)
	require.NoError(t, err)
	configPath := filepath.Join(root, "authorization.json")
	require.NoError(t, os.WriteFile(configPath, raw, 0600))
	settings := authorizationWorkspaceConfig("authz", "identity", "mapping")
	settingsRaw, err := json.Marshal(settings.Raw)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "config.yaml"), settingsRaw, 0600))
	options := ServeOptions{AuthzConfigPath: configPath, InternalResourceProviderIdentity: "host-resources", AuthorizationProviderFactory: func(context.Context, json.RawMessage, json.RawMessage) (AuthorizationProviders, error) {
		return AuthorizationProviders{Identity: principal, AllowsTenant: func(v string) bool { return v == "tenant" }}, nil
	}}
	registration := &internalHostRegistration{}
	configure, err := configureHostAuthorization(options, root, settings, nil, registration)
	require.NoError(t, err)
	require.Equal(t, root, registration.workspaceRoot)
	rt, backend, _, err := appserver.BuildWorkspaceRuntime(ctx, appserver.RuntimeOptions{WorkspaceRoot: root, Defaults: &execconfig.Defaults{}, SkipRegistryInitialize: true, ConfigureBuilder: configure})
	require.NoError(t, err)
	t.Cleanup(func() { _ = rt.Close(ctx) })
	require.NoError(t, registration.Bind(ctx, rt))
	namespaces, err := rt.PrimitiveProviders.Discover(ctx)
	require.NoError(t, err)
	var toolfeedNamespace *resources.DiscoveredNamespace
	for i := range namespaces {
		if namespaces[i].ProviderIdentity == "host-resources" && namespaces[i].Namespace == "team" {
			require.Nil(t, toolfeedNamespace, "duplicate owning toolfeed namespace")
			toolfeedNamespace = &namespaces[i]
		}
	}
	require.NotNil(t, toolfeedNamespace)
	rows, err := rt.PrimitiveProviders.List(ctx, "toolfeed", "team")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	got, err := rt.PrimitiveProviders.Get(ctx, toolfeedNamespace.Connection, identity.ResourceRef{URI: uri}, nil)
	require.NoError(t, err)
	require.Equal(t, "host-resources", got.ResolvedResource.ProviderIdentity)
	scoped, ok := backend.(interface {
		ListFeedSpecsContext(context.Context) ([]*sdk.FeedSpec, error)
		GetFeedSpecContext(context.Context, string) (*sdk.FeedSpec, error)
	})
	require.True(t, ok)
	specs, err := scoped.ListFeedSpecsContext(ctx)
	require.NoError(t, err)
	require.Len(t, specs, 1)
	require.Equal(t, "legacy-terminal", specs[0].ID)
	admitted, err := scoped.GetFeedSpecContext(ctx, "legacy-terminal")
	require.NoError(t, err)
	require.Equal(t, uri, admitted.URI)
	var definition api.FeedSpec
	require.NoError(t, json.Unmarshal(got.Resource.Definition, &definition))
	original := *admitted
	original.URI = ""
	original.ProviderIdentity = ""
	original.DefinitionFingerprint = ""
	original.DefinitionRevision = ""
	require.Equal(t, definition, original)
	principal.principal.Facts.Roles = nil
	_, err = rt.PrimitiveProviders.Get(ctx, namespaces[0].Connection, identity.ResourceRef{URI: got.Resource.URI}, nil)
	require.Error(t, err)
	options.InternalResourceProviderIdentity = ""
	_, err = configureHostAuthorization(options, root, settings, nil, registration)
	require.ErrorContains(t, err, "explicit internal resource provider")
}
