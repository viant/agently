package agently

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/viant/agently-core/app/executor"
	identity "github.com/viant/agently-core/protocol/resource"
	"github.com/viant/agently-core/service/policy"
	"github.com/viant/authz"
	"github.com/viant/authz/gating"
	"github.com/viant/authz/oauth"
	"github.com/viant/forge/backend/types"
)

func TestRemoteWindowAdmissionUsesExactRevisionAndCurrentHostPolicy(t *testing.T) {
	ctx := context.Background()
	uri := "window://external/catalog/orders"
	lease := time.Now().Add(time.Minute)
	principal := &reportHostIdentity{gating.Principal{Facts: authz.Facts{Subject: "alice", Issuer: "issuer", Tenant: "tenant", Roles: []string{"reader"}, ValidUntil: lease}, AccountID: "account", IdentityRevision: "one"}}
	var documents []authz.Document
	var requirements []gating.Binding
	for revision, role := range map[string]string{"1": "reader", "2": "admin"} {
		resource := authz.Resource{Kind: "window", ID: uri, Tenant: "tenant", Version: revision}
		documents = append(documents, authz.Document{Resource: resource, Revision: 1, Policies: map[string]authz.Policy{"open": {Mode: "protected", Rule: &authz.Rule{Kind: "role", Value: role}}}})
		requirements = append(requirements, gating.Binding{Resource: resource, Action: "open", Document: gating.RequirementsDocument{Revision: "one", Requirements: gating.Requirements{SchemaVersion: 1}}})
	}
	bundle, err := oauth.NewStaticAuthorization(oauth.StaticAuthorizationConfig{Identity: principal, AllowsTenant: func(tenant string) bool { return tenant == "tenant" }, Policies: documents, Requirements: requirements})
	require.NoError(t, err)
	prepared, err := executor.PrepareStaticAuthorization(executor.StaticAuthorizationRegistration{ProviderRef: "verified", CapabilityMappingRef: "explicit", PolicyVersion: "one", Service: bundle.ACL, Account: principal.Account, AuthorityRevision: principal.AuthorityRevision, AuthoritySnapshot: principal.ResolvePrincipal, GateEvaluator: bundle.Gates, BackendResources: []policy.BackendBinding{{Operation: "ui.window.open", ID: uri, UseResolvedResource: true, Resource: authz.Resource{Tenant: "tenant"}, Action: "open"}}})
	require.NoError(t, err)
	admit := configuredRemoteWindowAdmission(prepared)
	pin := identity.ResolvedResource{URI: uri, ProviderIdentity: "arbitrary-provider", ResourceCandidate: identity.ResourceCandidate{Kind: identity.StampedCandidate, Revision: "1", ContentFingerprint: strings.Repeat("a", 64)}, AuthorityBinding: "original-authority", ValidUntil: lease}
	window := &types.Window{}
	require.NoError(t, admit(ctx, pin, window))
	otherRevision := pin
	otherRevision.Revision = "2"
	require.Error(t, admit(ctx, otherRevision, window), "provider admission cannot replace the host's exact revision policy")
	unmapped := pin
	unmapped.URI = "window://external/catalog/unmapped"
	require.Error(t, admit(ctx, unmapped, window), "unknown external resources receive no implicit host grant")
	principal.principal.Facts.Roles = nil
	require.Error(t, admit(ctx, pin, window), "an earlier successful open does not freeze current role authority")
	principal.principal.Facts.Roles = []string{"reader"}
	expired := pin
	expired.ValidUntil = time.Now().Add(-time.Second)
	require.Error(t, admit(ctx, expired, window))
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	require.Error(t, admit(cancelled, pin, window))
	require.Error(t, admit(ctx, pin, nil))
	require.Error(t, configuredRemoteWindowAdmission(nil)(ctx, pin, window))
}
