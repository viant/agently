package agently

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/viant/agently-core/app/executor"
	corecfg "github.com/viant/agently-core/protocol/mcp/config"
	"github.com/viant/agently-core/protocol/mcp/manager"
	identity "github.com/viant/agently-core/protocol/resource"
	"github.com/viant/agently-core/service/policy"
	forge "github.com/viant/agently-core/service/primitiveprovider"
	"github.com/viant/agently-core/service/reporting"
	reportcatalog "github.com/viant/agently-core/service/reporting/catalog"
	resources "github.com/viant/agently-core/service/resource"
	windowloader "github.com/viant/agently-core/service/ui/window"
	"github.com/viant/authz"
	"github.com/viant/authz/gating"
	"github.com/viant/authz/oauth"
	"github.com/viant/forge/backend/types"
	mcp "github.com/viant/mcp"
	mcpclient "github.com/viant/mcp/client"
)

func TestInternalHostPreloadsNativeWindowsAndUsesMetadataOnlyList(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	write := func(name, body string) {
		file := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(file), 0700))
		require.NoError(t, os.WriteFile(file, []byte(body), 0600))
	}
	write("catalog.yaml", "baseURL: .\nwindows: []\n")
	write("extension/forge/reporting/.keep", "")
	config := &hostAuthorizationFile{CatalogPath: filepath.Join(root, "catalog.yaml")}
	principal := &reportHostIdentity{gating.Principal{Facts: authz.Facts{Subject: "alice", Issuer: "issuer", Tenant: "tenant", Roles: []string{"reader"}, ValidUntil: time.Now().Add(time.Minute)}, AccountID: "account-a", IdentityRevision: "identity-1"}}
	var selections []authz.SelectionDocument
	for i := 0; i < 20; i++ {
		key := fmt.Sprintf("window%d", i)
		uri := "window://platform/" + key
		write("extension/forge/windows/"+key+".yaml", "view:\n  title: "+key+"\n  content: {id: root}\n")
		config.WindowResources = append(config.WindowResources, windowloader.ResourceBinding{WindowKey: key, URI: uri})
		resource := authz.Resource{Kind: "window", ID: uri, Tenant: "tenant", Version: "working"}
		family := authz.ResourceFamily{Kind: "window", ID: uri, Tenant: "tenant"}
		config.Policies = append(config.Policies, authz.Document{Resource: resource, Revision: 1, Policies: map[string]authz.Policy{"read": {Mode: "protected", Rule: &authz.Rule{Kind: "role", Value: "reader"}}}})
		config.Requirements = append(config.Requirements, gating.Binding{Resource: resource, Action: "read", Document: gating.RequirementsDocument{Revision: "one", Requirements: gating.Requirements{SchemaVersion: 1}}})
		config.ResourceRevisionBindings = append(config.ResourceRevisionBindings, policy.ResourceRevisionBinding{Operation: policy.OperationWindowView, URI: uri, Resource: family, Action: "read"})
		selections = append(selections, authz.SelectionDocument{Resource: family, Revision: 1, DefaultVersion: "working"})
	}
	bundle, err := oauth.NewStaticAuthorization(oauth.StaticAuthorizationConfig{Identity: principal, AllowsTenant: func(s string) bool { return s == "tenant" }, Policies: config.Policies, Requirements: config.Requirements})
	require.NoError(t, err)
	mappings, err := authz.NewStaticSelectionStore(selections)
	require.NoError(t, err)
	prepared, err := executor.PrepareStaticAuthorization(executor.StaticAuthorizationRegistration{ProviderRef: "identity", CapabilityMappingRef: "mapping", PolicyVersion: "one", Service: bundle.ACL, Account: principal.Account, AuthorityRevision: principal.AuthorityRevision, AuthoritySnapshot: principal.ResolvePrincipal, GateEvaluator: bundle.Gates, ResourceRevisionMappings: mappings, ResourceRevisionBindings: config.ResourceRevisionBindings})
	require.NoError(t, err)
	snapshot, err := newInternalWindowSnapshot(ctx, root, "", config)
	require.NoError(t, err)
	require.EqualValues(t, 20, snapshot.CompileCount())
	resolveCalls := 0
	resolve := func(ctx context.Context, key string) (*identity.ResourceResolver, identity.ResourceRef, error) {
		resolveCalls++
		uri := key
		if key == "window0" {
			uri = "window://platform/window0"
		}
		r, err := prepared.ResourceResolver(policy.OperationWindowView, snapshot, nil)
		return r, identity.ResourceRef{URI: uri}, err
	}
	proof, err := types.NewWindowTargetHMAC(make([]byte, 32))
	require.NoError(t, err)
	registration := &internalHostRegistration{}
	local, err := configureInternalHostResources("internal", config, prepared, resolve, snapshot, nil, nil, nil, proof, principal, registration)
	require.NoError(t, err)
	mgr, err := manager.New(nil)
	require.NoError(t, err)
	require.NoError(t, mgr.RegisterLocal(ctx, "internal", &corecfg.MCPClient{ClientOptions: &mcp.ClientOptions{}, PrimitiveProviderIdentity: "internal"}, func(context.Context) (mcpclient.Interface, error) { return resources.NewLocalMCPClient(local) }))
	gateway := resources.NewGateway(mgr, prepared.ResourceActor, internalActorVerifier(prepared), "gateway")
	t.Cleanup(gateway.Close)
	rt := &executor.Runtime{PrimitiveProviders: gateway, UIBridge: forge.NewService(&forge.Config{})}
	require.NoError(t, registration.Bind(ctx, rt))
	for i := 0; i < 3; i++ {
		rows, err := rt.UIBridge.WindowDefinitionsList(ctx, nil)
		require.NoError(t, err)
		require.Len(t, rows.Windows, 20)
	}
	require.Zero(t, resolveCalls, "ungrouped listing must perform no resource resolver/selection/read")
	require.EqualValues(t, 20, snapshot.CompileCount(), "steady list must not compile YAML")
	got, err := rt.UIBridge.WindowDefinitionGet(ctx, &forge.WindowDefinitionGetInput{WindowID: "window0"})
	require.NoError(t, err)
	require.Equal(t, "internal", got.Definition.Resource.ProviderIdentity)
	require.Equal(t, "window://platform/window0", got.Definition.Resource.URI)
	originalURI, err := identity.ParseResourceURI(got.Definition.Resource.URI)
	require.NoError(t, err)
	require.NoError(t, snapshot.CheckCandidate(ctx, originalURI, got.Definition.Resource.ResourceCandidate))
	ref, err := rt.UIBridge.WindowResourceReference(ctx, "window0")
	require.NoError(t, err)
	require.Equal(t, got.Definition.Resource.URI, ref.URI)
	require.True(t, rt.UIBridge.WindowResourceConfigured("window0"))
	require.Greater(t, resolveCalls, 0, "direct get must retain resource admission")
	require.EqualValues(t, 20, snapshot.CompileCount())
	principal.principal.Facts.Roles = nil
	_, err = rt.UIBridge.WindowDefinitionGet(ctx, &forge.WindowDefinitionGetInput{WindowID: "window0", ResolvedResource: got.Definition.Resource, Target: got.Definition.ResourceTarget})
	require.Error(t, err, "direct read must deny revoked role even though ungrouped metadata remains listed")
	rows, err := rt.UIBridge.WindowDefinitionsList(ctx, nil)
	require.NoError(t, err)
	require.Len(t, rows.Windows, 20)
	principal.principal.Facts.Tenant = "foreign"
	rows, err = rt.UIBridge.WindowDefinitionsList(ctx, nil)
	if err == nil {
		require.Empty(t, rows.Windows, "explicit namespace must not leak across tenants")
	}
	write("extension/forge/windows/window0.yaml", "view:\n  title: Changed\n  content: {id: root}\n")
	_, err = snapshot.WindowIndex(ctx)
	require.ErrorIs(t, err, identity.ErrResourceStale)
	require.ErrorIs(t, snapshot.CheckCandidate(ctx, originalURI, got.Definition.Resource.ResourceCandidate), identity.ErrResourceStale)
	require.EqualValues(t, 20, snapshot.CompileCount(), "changed assets must not trigger hot reload")
	restarted, err := newInternalWindowSnapshot(ctx, root, "", config)
	require.NoError(t, err)
	rowsAfterRestart, err := restarted.WindowIndex(ctx)
	require.NoError(t, err)
	require.Len(t, rowsAfterRestart, 20)
}

func TestInternalHostReportReaderRetainsIndependentOperationPolicy(t *testing.T) {
	ctx := context.Background()
	uri := "report://platform/sales"
	principal := &reportHostIdentity{gating.Principal{Facts: authz.Facts{Subject: "alice", Issuer: "issuer", Tenant: "tenant", Roles: []string{"reader"}, ValidUntil: time.Now().Add(time.Minute)}, AccountID: "account-a", IdentityRevision: "one"}}
	family := authz.ResourceFamily{Kind: "report", ID: uri, Tenant: "tenant"}
	version := authz.Resource{Kind: "report", ID: uri, Tenant: "tenant", Version: "working"}
	config := &hostAuthorizationFile{CatalogPath: filepath.Join(t.TempDir(), "catalog.yaml"), ResourceRevisionBindings: []policy.ResourceRevisionBinding{{Operation: "report.retrieve", URI: uri, Resource: family, Action: "read"}, {Operation: "report.compile", URI: uri, Resource: family, Action: "read"}}}
	require.NoError(t, os.WriteFile(config.CatalogPath, []byte("windows: []\n"), 0600))
	bundle, err := oauth.NewStaticAuthorization(oauth.StaticAuthorizationConfig{Identity: principal, AllowsTenant: func(s string) bool { return s == "tenant" }, Policies: []authz.Document{{Resource: version, Revision: 1, Policies: map[string]authz.Policy{"read": {Mode: "protected", Rule: &authz.Rule{Kind: "role", Value: "reader"}}}}}, Requirements: []gating.Binding{{Resource: version, Action: "read", Document: gating.RequirementsDocument{Revision: "one", Requirements: gating.Requirements{SchemaVersion: 1}}}}})
	require.NoError(t, err)
	mappings, err := authz.NewStaticSelectionStore([]authz.SelectionDocument{{Resource: family, Revision: 1, DefaultVersion: "working"}})
	require.NoError(t, err)
	prepared, err := executor.PrepareStaticAuthorization(executor.StaticAuthorizationRegistration{ProviderRef: "identity", CapabilityMappingRef: "mapping", PolicyVersion: "one", Service: bundle.ACL, Account: principal.Account, AuthorityRevision: principal.AuthorityRevision, AuthoritySnapshot: principal.ResolvePrincipal, GateEvaluator: bundle.Gates, ResourceRevisionMappings: mappings, ResourceRevisionBindings: config.ResourceRevisionBindings})
	require.NoError(t, err)
	source := &reportHostSource{raw: json.RawMessage(`{"schemaVersion":1,"reportDocument":{"title":"Sales"},"reportSpec":{"version":1,"kind":"reportSpec","source":{"kind":"dashboard.reportBuilder","containerId":"demo","stateKey":"demo","dataSourceRef":"demo"},"title":"Sales","parameters":{"viewMode":"table","groupBy":"","pageSize":25,"orderField":"","orderDir":"asc"},"layoutIntent":{"kind":"single","resultPanePosition":"left","blockOrder":["primaryTable"]},"refinements":[],"calculatedFields":[],"datasets":[{"id":"primary","dataSourceRef":"demo","request":{}}],"blocks":[{"id":"primaryTable","kind":"tableBlock","datasetRef":"primary","columns":[]}]}}`)}
	reg := &internalHostRegistration{}
	local, err := configureInternalHostResources("internal", config, prepared, nil, nil, &internalReportRegistration{bindings: []resources.LocalResourceBinding{{URI: uri, Title: "Sales", FormatVersion: 1, Resolver: func(ctx context.Context, _ identity.VerifiedActor, _ string) (*identity.ResourceResolver, error) {
		return prepared.ResourceResolver("report.retrieve", source, nil)
	}}}}, &reportcatalog.ReportCatalogService{}, nil, nil, principal, reg)
	require.NoError(t, err)
	mgr, err := manager.New(nil)
	require.NoError(t, err)
	require.NoError(t, mgr.RegisterLocal(ctx, "internal", &corecfg.MCPClient{ClientOptions: &mcp.ClientOptions{}, PrimitiveProviderIdentity: "internal"}, func(context.Context) (mcpclient.Interface, error) { return resources.NewLocalMCPClient(local) }))
	gateway := resources.NewGateway(mgr, prepared.ResourceActor, internalActorVerifier(prepared), "gateway")
	t.Cleanup(gateway.Close)
	rt := &executor.Runtime{PrimitiveProviders: gateway, Reporting: reporting.New(reporting.Options{Store: reporting.NewMemoryStore(), Compiler: reporting.NewReportSpecCompiler(time.Now)})}
	require.NoError(t, reg.Bind(ctx, rt))
	list, err := rt.Reporting.ListReports(ctx, &reporting.ListReportsInput{})
	require.NoError(t, err)
	require.Len(t, list.Reports, 1)
	entry := list.Reports[0]
	require.Equal(t, "internal", entry.Resource.ProviderIdentity)
	require.True(t, entry.Capabilities.Open)
	require.False(t, entry.Capabilities.Run)
	short := time.Now().Add(10 * time.Second)
	principal.principal.Facts.ValidUntil = short
	lease, err := internalOperationLeaseAdmission(prepared, "internal")(ctx, "report.compile", *entry.Resource, source.raw)
	require.NoError(t, err)
	require.True(t, lease.Equal(short), "host admission must return the verified shorter authority deadline")
	capped := *entry.Resource
	capped.ValidUntil = lease
	principal.principal.Facts.ValidUntil = time.Now().Add(time.Minute)
	lease, err = internalOperationLeaseAdmission(prepared, "internal")(ctx, "report.compile", capped, source.raw)
	require.NoError(t, err)
	require.True(t, lease.Equal(short), "a regrant cannot extend an already captured operation lease")
	_, err = rt.Reporting.GetReport(ctx, &reporting.GetReportInput{ResolvedResource: entry.Resource})
	require.NoError(t, err)
	_, err = rt.Reporting.Compile(ctx, &reporting.CompileRequest{ResolvedResource: entry.Resource})
	require.NoError(t, err)
	principal.principal.Facts.Roles = nil
	_, err = rt.Reporting.GetReport(ctx, &reporting.GetReportInput{ResolvedResource: entry.Resource})
	require.Error(t, err)
}
