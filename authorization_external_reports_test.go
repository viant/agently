package agently

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/viant/agently-core/genai/llm"
	agentproto "github.com/viant/agently-core/protocol/agent"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/viant/agently-core/app/executor"
	execconfig "github.com/viant/agently-core/app/executor/config"
	mcpcfg "github.com/viant/agently-core/protocol/mcp/config"
	identity "github.com/viant/agently-core/protocol/resource"
	requestctx "github.com/viant/agently-core/runtime/requestctx"
	authsvc "github.com/viant/agently-core/service/auth"
	"github.com/viant/agently-core/service/policy"
	reporting "github.com/viant/agently-core/service/reporting"
	catalog "github.com/viant/agently-core/service/reporting/catalog"
	resources "github.com/viant/agently-core/service/resource"
	"github.com/viant/authz"
	"github.com/viant/authz/gating"
	"github.com/viant/authz/oauth"
	mcp "github.com/viant/mcp"
	mcpclient "github.com/viant/mcp/client"
)

func TestHostedReportAdmissionRetainsLocalPolicyAndForeignProviderPin(t *testing.T) {
	ctx := authsvc.InjectUser(context.Background(), "alice")
	uri := "report://host/local"
	owner := "internal"
	family := authz.ResourceFamily{Kind: "report", ID: uri, Tenant: "tenant"}
	resource := authz.Resource{Kind: "report", ID: uri, Tenant: "tenant", Version: "working"}
	principal := &reportHostIdentity{gating.Principal{Facts: authz.Facts{Subject: "alice", Issuer: "issuer", Tenant: "tenant", Roles: []string{"reader"}, ValidUntil: time.Now().Add(10 * time.Minute)}, AccountID: "account-a", IdentityRevision: "identity-1"}}
	docs := []authz.Document{{Resource: resource, Revision: 1, Policies: map[string]authz.Policy{"read": {Mode: "protected", Rule: &authz.Rule{Kind: "role", Value: "reader"}}, "write": {Mode: "protected", Rule: &authz.Rule{Kind: "role", Value: "admin"}}}}}
	requirements := []gating.Binding{}
	for _, action := range []string{"read", "write"} {
		requirements = append(requirements, gating.Binding{Resource: resource, Action: action, Document: gating.RequirementsDocument{Revision: "one", Requirements: gating.Requirements{SchemaVersion: 1}}})
	}
	bundle, err := oauth.NewStaticAuthorization(oauth.StaticAuthorizationConfig{Identity: principal, AllowsTenant: func(s string) bool { return s == "tenant" }, Policies: docs, Requirements: requirements})
	require.NoError(t, err)
	selection, err := authz.NewStaticSelectionStore([]authz.SelectionDocument{{Resource: family, Revision: 1, DefaultVersion: "working"}})
	require.NoError(t, err)
	bindings := []policy.ResourceRevisionBinding{}
	for _, op := range []string{"report.retrieve", "report.compile", "report.execute", "report.export"} {
		action := "read"
		if op == "report.execute" || op == "report.export" {
			action = "write"
		}
		bindings = append(bindings, policy.ResourceRevisionBinding{Operation: op, URI: uri, Resource: family, Action: action})
	}
	prepared, err := executor.PrepareStaticAuthorization(executor.StaticAuthorizationRegistration{ProviderRef: "verified", CapabilityMappingRef: "explicit", PolicyVersion: "one", Service: bundle.ACL, Account: principal.Account, AuthorityRevision: principal.AuthorityRevision, AuthoritySnapshot: principal.ResolvePrincipal, GateEvaluator: bundle.Gates, ResourceRevisionMappings: selection, ResourceRevisionBindings: bindings})
	require.NoError(t, err)
	raw := json.RawMessage(`{"schemaVersion":1,"reportDocument":{"title":"Local"},"reportSpec":{"version":1,"kind":"reportSpec","source":{"kind":"dashboard.reportBuilder","containerId":"local","stateKey":"local","dataSourceRef":"local"},"title":"Local","parameters":{"viewMode":"table","groupBy":"","pageSize":25,"orderField":"","orderDir":"asc"},"layoutIntent":{"kind":"single","resultPanePosition":"left","blockOrder":["primaryTable"]},"refinements":[],"calculatedFields":[],"datasets":[{"id":"primary","dataSourceRef":"demo","request":{}}],"blocks":[{"id":"primaryTable","kind":"tableBlock","datasetRef":"primary","columns":[]}]} ,"dataSources":{"demo":{"id":"demo","cardinality":"collection","backend":{"kind":"inline","rows":[{"id":"one"}]}}}}`)
	source := &reportHostSource{raw: raw}
	resolver, err := prepared.ResourceResolver("report.retrieve", source, nil)
	require.NoError(t, err)
	resolver.ProviderIdentity = owner
	localPin, err := resolver.Resolve(ctx, identity.ResourceRef{URI: uri})
	require.NoError(t, err)
	foreignPin := *localPin
	foreignPin.URI = "report://external/foreign"
	foreignPin.ProviderIdentity = "external"
	foreignPin.AuthorityBinding = "foreign-provider-binding"
	called := 0
	hostPolicy := func(ctx context.Context, op, id string) error {
		called++
		pin, ok := requestctx.ResolvedResourceFromContext(ctx)
		require.True(t, ok)
		require.Equal(t, foreignPin.URI, pin.URI)
		require.Equal(t, foreignPin.ProviderIdentity, pin.ProviderIdentity)
		require.Equal(t, foreignPin.ResourceCandidate, pin.ResourceCandidate)
		require.Equal(t, foreignPin.AuthorityBinding, pin.AuthorityBinding)
		require.True(t, foreignPin.ValidUntil.Equal(pin.ValidUntil))
		require.Equal(t, foreignPin.URI, id)
		if op == "report.export" {
			return identity.ErrResourceDenied
		}
		return nil
	}
	admission := hostedReportOperationAdmission(prepared, owner, hostPolicy)
	_, err = admission(ctx, "report.execute", *localPin, raw)
	require.Error(t, err)
	require.Zero(t, called, "local policy must not be replaced by foreign admission")
	deadline, err := admission(ctx, "report.execute", foreignPin, raw)
	require.NoError(t, err)
	require.Equal(t, foreignPin.ValidUntil, deadline)
	_, err = admission(ctx, "report.export", foreignPin, raw)
	require.ErrorIs(t, err, identity.ErrResourceDenied)
	caps := hostedReportCapabilities(prepared, owner, hostPolicy)
	localCaps, err := caps(ctx, identity.VerifiedActor{}, catalog.ReportCatalogCandidate{}, *localPin)
	require.NoError(t, err)
	require.True(t, localCaps.Open)
	require.False(t, localCaps.Run)
	require.False(t, localCaps.Export)
	foreignCaps, err := caps(ctx, identity.VerifiedActor{}, catalog.ReportCatalogCandidate{}, foreignPin)
	require.NoError(t, err)
	require.True(t, foreignCaps.Open)
	require.True(t, foreignCaps.Run)
	require.False(t, foreignCaps.Export)
	require.False(t, foreignCaps.Edit)
	require.False(t, foreignCaps.Delete)

	// Exercise the actual late binder with the catalog supplied by the ordinary
	// Builder; no reader or dataset executor is installed by the test itself.
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "catalog.yaml"), []byte("baseURL: .\nwindows: []\n"), 0600))
	config := &hostAuthorizationFile{CatalogPath: filepath.Join(root, "catalog.yaml"), ResourceRevisionBindings: bindings}
	localCatalog := &catalog.ReportCatalogService{Identity: prepared.ResourceActor, Inventories: []catalog.ReportInventory{catalog.ReportInventoryFunc(func(context.Context, identity.VerifiedActor) ([]catalog.ReportCatalogCandidate, error) {
		return []catalog.ReportCatalogCandidate{{URI: uri, Title: "Local"}}, nil
	})}, Resolver: func(context.Context) (*identity.ResourceResolver, error) { return resolver, nil }}
	registration := &internalHostRegistration{}
	reportRegistration := &internalReportRegistration{bindings: []resources.LocalResourceBinding{{URI: uri, Title: "Local", FormatVersion: 1, Resolver: func(context.Context, identity.VerifiedActor, string) (*identity.ResourceResolver, error) {
		return resolver, nil
	}}}}
	local, err := configureInternalHostResources(owner, config, prepared, nil, nil, reportRegistration, localCatalog, nil, nil, principal, registration)
	require.NoError(t, err)
	runtime, err := executor.NewBuilder().WithModelFinder(reportCompositionModelFinder{}).WithAgentFinder(reportCompositionAgentFinder{}).WithLocalPrimitiveProvider(local).WithPrimitiveAuthority(prepared.ResourceActor, internalActorVerifier(prepared), owner).WithReportResources(localCatalog, func(context.Context, string) (*identity.ResourceResolver, error) { return resolver, nil }).WithReportingService(reporting.New(reporting.Options{Store: reporting.NewMemoryStore(), Compiler: reporting.NewReportSpecCompiler(time.Now), Exporter: reporting.NewForgeExporter(nil)})).WithDefaults(&execconfig.Defaults{Reporting: execconfig.ReportingDefaults{Enabled: true, QueueIntervalMs: 60000}}).Build(ctx)
	require.NoError(t, err)
	defer runtime.Close(ctx)
	foreignURI, _ := identity.ParseResourceURI(foreignPin.URI)
	foreignRaw := append(json.RawMessage(nil), raw...)
	foreign, err := resources.NewLocalProvider(resources.LocalConfig{ProviderIdentity: "external", Actor: prepared.ResourceActor, Verify: internalActorVerifier(prepared), Authorize: func(_ context.Context, _ identity.VerifiedActor, u identity.ResourceURI, _ string) error {
		if u.Namespace != foreignURI.Namespace {
			return identity.ErrResourceDenied
		}
		return nil
	}, Validators: map[string]resources.LocalResourceValidator{"report": resources.ValidateReportEnvelope}, Bindings: []resources.LocalResourceBinding{{URI: foreignPin.URI, Title: "Foreign", FormatVersion: 1, Resolver: func(context.Context, identity.VerifiedActor, string) (*identity.ResourceResolver, error) {
		return &identity.ResourceResolver{ProviderIdentity: "external", Source: &identity.LocalResource{URI: foreignURI, Load: func(context.Context) (json.RawMessage, error) { return foreignRaw, nil }}, Policy: externalHostReportPolicy{binding: foreignPin.AuthorityBinding, until: foreignPin.ValidUntil}}, nil
	}}}})
	require.NoError(t, err)
	require.NoError(t, runtime.MCPManager.RegisterLocal(ctx, "foreign-reports", &mcpcfg.MCPClient{ClientOptions: &mcp.ClientOptions{}, PrimitiveProviderIdentity: "external"}, func(context.Context) (mcpclient.Interface, error) { return resources.NewLocalMCPClient(foreign) }))
	runtime.ReportAuthorizer = hostPolicy
	require.NoError(t, registration.Bind(ctx, runtime))
	foreignLocated, lookupErr := runtime.PrimitiveProviders.GetForProvider(ctx, "external", identity.ResourceRef{URI: foreignPin.URI}, nil)
	require.NoError(t, lookupErr, "actual external provider retrieval")
	require.NotNil(t, foreignLocated.ResolvedResource)
	foreignGet, getErr := runtime.Reporting.GetReport(ctx, &reporting.GetReportInput{Resource: &identity.ResourceRef{URI: foreignPin.URI}})
	require.NoError(t, getErr, "configured reporting reader retrieval")
	require.NotNil(t, foreignGet)
	foreignRun, err := runtime.Reporting.ExecuteResource(ctx, &reporting.ExecuteResourceRequest{Resource: &identity.ResourceRef{URI: foreignPin.URI}})
	require.NoError(t, err)
	require.Equal(t, "external", foreignRun.Resource.ProviderIdentity)
	require.Equal(t, foreignPin.AuthorityBinding, foreignRun.Resource.AuthorityBinding)
	reportList, err := runtime.Reporting.ListReports(ctx, &reporting.ListReportsInput{Namespace: "external"})
	require.NoError(t, err)
	require.Len(t, reportList.Reports, 1)
	require.True(t, reportList.Reports[0].Capabilities.Open)
	require.True(t, reportList.Reports[0].Capabilities.Run)
	require.False(t, reportList.Reports[0].Capabilities.Export)
	require.False(t, reportList.Reports[0].Capabilities.Edit)
	_, err = runtime.Reporting.ExecuteResource(ctx, &reporting.ExecuteResourceRequest{ResolvedResource: localPin})
	require.Error(t, err, "unchanged local execute denial must survive actual late registration")
	legacy, err := executor.NewBuilder().WithModelFinder(reportCompositionModelFinder{}).WithAgentFinder(reportCompositionAgentFinder{}).WithPrimitiveAuthority(prepared.ResourceActor, internalActorVerifier(prepared), owner).WithReportResources(localCatalog, func(_ context.Context, operation string) (*identity.ResourceResolver, error) {
		r, e := prepared.ResourceResolver(operation, source, nil)
		if r != nil {
			r.ProviderIdentity = owner
		}
		return r, e
	}).WithReportingService(reporting.New(reporting.Options{Store: reporting.NewMemoryStore(), Compiler: reporting.NewReportSpecCompiler(time.Now), Exporter: reporting.NewForgeExporter(nil)})).WithDefaults(&execconfig.Defaults{Reporting: execconfig.ReportingDefaults{Enabled: true, QueueIntervalMs: 60000}}).Build(ctx)
	require.NoError(t, err)
	defer legacy.Close(ctx)
	require.NoError(t, legacy.MCPManager.RegisterLocal(ctx, "foreign-reports", &mcpcfg.MCPClient{ClientOptions: &mcp.ClientOptions{}, PrimitiveProviderIdentity: "external"}, func(context.Context) (mcpclient.Interface, error) { return resources.NewLocalMCPClient(foreign) }))
	_, err = legacy.Reporting.ExecuteResource(ctx, &reporting.ExecuteResourceRequest{Resource: &identity.ResourceRef{URI: uri}})
	require.Error(t, err, "legacy local denial must not fall through to a foreign source")
	legacyForeign, err := legacy.Reporting.ExecuteResource(ctx, &reporting.ExecuteResourceRequest{Resource: &identity.ResourceRef{URI: foreignPin.URI}})
	require.NoError(t, err)
	require.Equal(t, "external", legacyForeign.Resource.ProviderIdentity)
	legacyList, err := legacy.Reporting.ListReports(ctx, &reporting.ListReportsInput{})
	require.NoError(t, err)
	require.Len(t, legacyList.Reports, 2)
	shadowURI, _ := identity.ParseResourceURI(uri)
	shadowGets := 0
	shadow, err := resources.NewLocalProvider(resources.LocalConfig{ProviderIdentity: "shadow", Actor: prepared.ResourceActor, Verify: internalActorVerifier(prepared), Authorize: func(_ context.Context, _ identity.VerifiedActor, u identity.ResourceURI, _ string) error {
		if u.Namespace != shadowURI.Namespace {
			return identity.ErrResourceDenied
		}
		return nil
	}, Validators: map[string]resources.LocalResourceValidator{"report": resources.ValidateReportEnvelope}, Bindings: []resources.LocalResourceBinding{{URI: uri, Title: "Shadow", FormatVersion: 1, Resolver: func(context.Context, identity.VerifiedActor, string) (*identity.ResourceResolver, error) {
		shadowGets++
		return &identity.ResourceResolver{ProviderIdentity: "shadow", Source: &identity.LocalResource{URI: shadowURI, Load: func(context.Context) (json.RawMessage, error) { return raw, nil }}, Policy: externalHostReportPolicy{binding: "shadow-binding", until: foreignPin.ValidUntil}}, nil
	}}}})
	require.NoError(t, err)
	require.NoError(t, legacy.MCPManager.RegisterLocal(ctx, "shadow-reports", &mcpcfg.MCPClient{ClientOptions: &mcp.ClientOptions{}, PrimitiveProviderIdentity: "shadow"}, func(context.Context) (mcpclient.Interface, error) { return resources.NewLocalMCPClient(shadow) }))
	_, err = legacy.Reporting.ExecuteResource(ctx, &reporting.ExecuteResourceRequest{Resource: &identity.ResourceRef{URI: uri}})
	require.Error(t, err)
	require.Zero(t, shadowGets, "denied local ownership must never become foreign same-URI dispatch")
	_, err = legacy.Reporting.ListReports(ctx, &reporting.ListReportsInput{})
	require.Error(t, err, "ambiguous local/gateway ownership must not publish a merged alias")

}

type reportCompositionModelFinder struct{}

func (reportCompositionModelFinder) Find(context.Context, string) (llm.Model, error) {
	return nil, fmt.Errorf("no model execution in report composition fixture")
}

type reportCompositionAgentFinder struct{}

func (reportCompositionAgentFinder) Find(context.Context, string) (*agentproto.Agent, error) {
	return nil, fmt.Errorf("no agent execution in report composition fixture")
}

type externalHostReportPolicy struct {
	binding string
	until   time.Time
}

func (p externalHostReportPolicy) SelectRevision(_ context.Context, _ identity.ResourceRef, c []identity.ResourceCandidate) (identity.ResourceDecision, error) {
	if len(c) != 1 {
		return identity.ResourceDecision{}, identity.ErrResourceDenied
	}
	return identity.ResourceDecision{Candidate: c[0], AuthorityBinding: p.binding, ValidUntil: p.until}, nil
}
