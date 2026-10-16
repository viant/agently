package agently

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	identity "github.com/viant/agently-core/protocol/resource"
)

func TestWorkspaceReportSnapshotIsStaticAndUsesFreshOperationResolver(t *testing.T) {
	root := t.TempDir()
	writeInternalReportAsset(t, root, "extension/forge/reporting/builder.yaml", `kind: forge.reporting.builder
id: metrics
reportBuilder:
  document: {title: Baseline}
`)
	writeInternalReportAsset(t, root, "extension/forge/reporting/delivery.yaml", `kind: forge.reporting.report
id: delivery
namespace: analytics
name: delivery
resourceUri: report://analytics/delivery
builderRef: metrics
label: Delivery
document: {title: Delivery, blocks: [{id: intro, kind: markdownBlock, markdown: authored}]}
`)
	writeInternalReportAsset(t, root, "extension/forge/windows/reports/overview/main.yaml", `windowKey: reports/overview
view:
  content: {id: reports-root}
`)

	var actorCalls, resolverCalls int
	var operations []string
	actor := identity.VerifiedActor{Subject: "synthetic-user", Issuer: "https://issuer.fixture", TenantID: "analytics", AccountID: "opaque-account", IdentityRevision: "facts-v1", ValidUntil: time.Now().Add(time.Minute)}
	access := HostResourceAccess{
		ProviderIdentity: "internal",
		Actor: func(context.Context) (identity.VerifiedActor, error) {
			actorCalls++
			return actor, nil
		},
		Resolver: func(_ context.Context, operation string, source identity.ResourceSource) (*identity.ResourceResolver, error) {
			resolverCalls++
			operations = append(operations, operation)
			policy := internalReportTestRevisionPolicy{actor: actor}
			return &identity.ResourceResolver{ProviderIdentity: "internal", Source: source, Policy: policy}, nil
		},
	}
	config := json.RawMessage(`{"source":"workspace","builderWindows":{"metrics":"reports/overview"}}`)
	providers, err := workspaceReportResourceProviders(context.Background(), config, root, access)
	require.NoError(t, err)
	require.Len(t, providers.LocalBindings, 1)
	require.Zero(t, actorCalls, "startup snapshotting does not resolve a principal")
	require.Zero(t, resolverCalls, "startup snapshotting does not build request authority")

	source, err := providers.Source(context.Background(), actor)
	require.NoError(t, err)
	uri, err := identity.ParseResourceURI("report://analytics/delivery")
	require.NoError(t, err)
	candidates, err := source.Candidates(context.Background(), uri)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	first, err := source.ReadCandidate(context.Background(), uri, candidates[0])
	require.NoError(t, err)
	secondCandidates, err := source.Candidates(context.Background(), uri)
	require.NoError(t, err)
	require.Len(t, secondCandidates, 1)
	second, err := source.ReadCandidate(context.Background(), uri, secondCandidates[0])
	require.NoError(t, err)
	require.Equal(t, candidates[0].ContentFingerprint, secondCandidates[0].ContentFingerprint)
	require.Equal(t, first, second, "a restarted read of the native snapshot has the exact same materialized bytes")
	require.Equal(t, identity.ContentFingerprint(first), candidates[0].ContentFingerprint)
	require.Zero(t, actorCalls)
	require.Zero(t, resolverCalls)

	// The generic resource boundary invokes the registered resolver afresh for
	// each real operation. The native report policy starts from retrieve access;
	// report execute/export have their own pinned action checks in the gateway.
	for _, action := range []string{"resource.get", "resource.list"} {
		resolver, resolveErr := providers.LocalBindings[0].Resolver(context.Background(), actor, action)
		require.NoError(t, resolveErr)
		pin, resolveErr := resolver.Resolve(context.Background(), identity.ResourceRef{URI: uri.String()})
		require.NoError(t, resolveErr)
		resolvedBytes, _, readErr := resolver.ReadResolved(context.Background(), *pin)
		require.NoError(t, readErr)
		require.Equal(t, first, resolvedBytes)
	}
	require.Equal(t, 2, resolverCalls)
	require.Equal(t, []string{"report.retrieve", "report.retrieve"}, operations)
	require.Zero(t, actorCalls, "the supplied verified actor is consumed by the caller, not reloaded during snapshot reads")
	access.Resolver = func(context.Context, string, identity.ResourceSource) (*identity.ResourceResolver, error) {
		return nil, identity.ErrResourceDenied
	}
	denied, err := workspaceReportResourceProviders(context.Background(), config, root, access)
	require.NoError(t, err)
	_, err = denied.LocalBindings[0].Resolver(context.Background(), actor, "resource.get")
	require.ErrorIs(t, err, identity.ErrResourceDenied, "cached content cannot bypass fresh operation authority")

}

type internalReportTestRevisionPolicy struct{ actor identity.VerifiedActor }

func (p internalReportTestRevisionPolicy) SelectRevision(_ context.Context, ref identity.ResourceRef, candidates []identity.ResourceCandidate) (identity.ResourceDecision, error) {
	if ref.URI != "report://analytics/delivery" || len(candidates) != 1 || candidates[0].Kind != identity.WorkingCandidate {
		return identity.ResourceDecision{}, identity.ErrResourceDenied
	}
	return identity.ResourceDecision{Candidate: candidates[0], AuthorityBinding: "opaque-account/facts-v1", ValidUntil: p.actor.ValidUntil}, nil
}

func writeInternalReportAsset(t *testing.T, root, name, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	require.NoError(t, os.WriteFile(path, []byte(body), 0600))
}

func TestWorkspaceReportProviderRejectsUnconfiguredSourcesAndAuthority(t *testing.T) {
	for _, raw := range []string{
		`{"source":"sqlite","database":"resources.sqlite"}`,
		`{"source":"unknown","builderWindows":{"metrics":"reports/overview"}}`,
		`{"source":"workspace","builderWindows":{"metrics":"reports/overview"},"unknown":true}`,
		`{"source":"workspace","builderWindows":{"metrics":"reports/overview"}} {}`,
		`{"source":"workspace"}`,
	} {
		t.Run(raw, func(t *testing.T) {
			_, err := workspaceReportResourceProviders(context.Background(), json.RawMessage(raw), t.TempDir(), HostResourceAccess{})
			require.Error(t, err)
		})
	}
	_, err := workspaceReportResourceProviders(context.Background(), json.RawMessage(`{"source":"workspace","builderWindows":{"metrics":"reports/overview"}}`), t.TempDir(), HostResourceAccess{ProviderIdentity: "internal"})
	require.ErrorContains(t, err, "explicit host authority")
}
