package agently

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/viant/agently-core/app/executor"
	identity "github.com/viant/agently-core/protocol/resource"
	"github.com/viant/agently-core/service/policy"
	"github.com/viant/agently-core/service/reporting"
	"github.com/viant/agently-core/service/reporting/catalog"
	"github.com/viant/authz"
	"github.com/viant/authz/gating"
	"github.com/viant/authz/oauth"
)

type reportHostIdentity struct{ principal gating.Principal }

func (p *reportHostIdentity) Resolve(context.Context) (authz.Facts, error) {
	return p.principal.Facts, nil
}
func (p *reportHostIdentity) ResolvePrincipal(context.Context) (gating.Principal, error) {
	return p.principal, nil
}
func (p *reportHostIdentity) Account(context.Context, authz.Facts) (string, error) {
	return p.principal.AccountID, nil
}
func (p *reportHostIdentity) AuthorityRevision(context.Context, authz.Facts, string) (string, time.Time, error) {
	return p.principal.IdentityRevision, p.principal.Facts.ValidUntil, nil
}

type reportHostSource struct{ raw json.RawMessage }

func (s *reportHostSource) Candidates(context.Context, identity.ResourceURI) ([]identity.ResourceCandidate, error) {
	return []identity.ResourceCandidate{{Kind: identity.WorkingCandidate, ContentFingerprint: identity.ContentFingerprint(s.raw)}}, nil
}
func (s *reportHostSource) ReadCandidate(context.Context, identity.ResourceURI, identity.ResourceCandidate) (json.RawMessage, error) {
	return s.raw, nil
}

func TestHostReportResourcesUseCurrentIdentityAndActionPolicy(t *testing.T) {
	ctx := context.Background()
	uri := "report://analytics/sales"
	family := authz.ResourceFamily{Kind: "report", ID: uri, Tenant: "tenant"}
	resource := authz.Resource{Kind: "report", ID: uri, Tenant: "tenant", Version: "working"}
	principal := &reportHostIdentity{gating.Principal{Facts: authz.Facts{Subject: "alice", Issuer: "issuer", Tenant: "tenant", Roles: []string{"reader"}, ValidUntil: time.Now().Add(time.Minute)}, AccountID: "account-a", IdentityRevision: "identity-1"}}
	bundle, err := oauth.NewStaticAuthorization(oauth.StaticAuthorizationConfig{Identity: principal, AllowsTenant: func(v string) bool { return v == "tenant" }, Policies: []authz.Document{{Resource: resource, Revision: 1, Policies: map[string]authz.Policy{"read": {Mode: "protected", Rule: &authz.Rule{Kind: "role", Value: "reader"}}}}}, Requirements: []gating.Binding{{Resource: resource, Action: "read", Document: gating.RequirementsDocument{Revision: "requirements-1", Requirements: gating.Requirements{SchemaVersion: 1}}}}})
	if err != nil {
		t.Fatal(err)
	}
	selections, err := authz.NewStaticSelectionStore([]authz.SelectionDocument{{Resource: family, Revision: 1, DefaultVersion: "working"}})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := executor.PrepareStaticAuthorization(executor.StaticAuthorizationRegistration{ProviderRef: "verified", CapabilityMappingRef: "explicit", PolicyVersion: "policy-1", Service: bundle.ACL, Account: principal.Account, AuthorityRevision: principal.AuthorityRevision, AuthoritySnapshot: principal.ResolvePrincipal, GateEvaluator: bundle.Gates, ResourceRevisionMappings: selections, ResourceRevisionBindings: []policy.ResourceRevisionBinding{{Operation: "report.retrieve", URI: uri, Resource: family, Action: "read"}}})
	if err != nil {
		t.Fatal(err)
	}
	source := &reportHostSource{json.RawMessage(`{"schemaVersion":1,"reportDocument":{"title":"Sales"},"reportSpec":{"title":"Sales"}}`)}
	var sourceActor identity.VerifiedActor
	factory := func(_ context.Context, raw json.RawMessage, workspace string, access HostResourceAccess) (ReportResourceProviders, error) {
		if string(raw) != `{"storage":"fixture"}` || workspace != "trusted-workspace" {
			t.Fatal("operator settings changed")
		}
		return ReportResourceProviders{Source: func(_ context.Context, actor identity.VerifiedActor) (identity.ResourceSource, error) {
			sourceActor = actor
			return source, nil
		}, Inventories: []catalog.ReportInventory{catalog.ReportInventoryFunc(func(context.Context, identity.VerifiedActor) ([]catalog.ReportCatalogCandidate, error) {
			return []catalog.ReportCatalogCandidate{{URI: uri, Title: "Sales", OwnerID: "alice"}}, nil
		})}}, nil
	}
	catalog, resolve, _, _, err := configureHostReportResources(ServeOptions{ReportResourceProviderFactory: factory}, &hostAuthorizationFile{ReportResources: json.RawMessage(`{"storage":"fixture"}`)}, "trusted-workspace", prepared, principal)
	if err != nil {
		t.Fatal(err)
	}
	service := reporting.New(reporting.Options{ReportCatalog: catalog, ResourceResolver: resolve, Store: reporting.NewMemoryStore()})
	list, err := service.ListReports(ctx, &reporting.ListReportsInput{CurrentUserOnly: true})
	if err != nil || len(list.Reports) != 1 {
		t.Fatalf("list: %+v %v", list, err)
	}
	entry := list.Reports[0]
	if sourceActor.Subject != "alice" || sourceActor.AccountID != "account-a" || !entry.OwnedByCurrentUser || entry.Capabilities == nil || !entry.Capabilities.Open || entry.Capabilities.Run || entry.Capabilities.Edit || entry.Capabilities.Export {
		t.Fatalf("authority or capabilities lost: %+v %+v", sourceActor, entry)
	}
	if _, err = service.GetReport(ctx, &reporting.GetReportInput{ResolvedResource: entry.Resource}); err != nil {
		t.Fatal(err)
	}
	principal.principal.AccountID = "account-b"
	if _, err = service.GetReport(ctx, &reporting.GetReportInput{ResolvedResource: entry.Resource}); err == nil {
		t.Fatal("opened pin crossed account boundary")
	}
	principal.principal.AccountID = "account-a"
	principal.principal.Facts.Roles = nil
	list, err = service.ListReports(ctx, &reporting.ListReportsInput{})
	if err != nil || len(list.Reports) != 0 {
		t.Fatalf("restricted metadata visible: %+v %v", list, err)
	}
	principal.principal.Facts.ValidUntil = time.Now().Add(-time.Second)
	if _, err = service.ListReports(ctx, &reporting.ListReportsInput{}); err == nil {
		t.Fatal("expired identity accepted")
	}
}

func TestHostReportResourcesRequireExplicitStorageBinding(t *testing.T) {
	_, _, _, _, err := configureHostReportResources(ServeOptions{}, &hostAuthorizationFile{ReportResources: json.RawMessage(`{}`)}, "workspace", nil, nil)
	if err == nil {
		t.Fatal("configured resources silently fell back to legacy storage")
	}
}
