package agently

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	execconfig "github.com/viant/agently-core/app/executor/config"
	appserver "github.com/viant/agently-core/app/server"
	"github.com/viant/agently-core/service/reporting"
	"github.com/viant/agently-core/workspace"
	datasource "github.com/viant/forge/backend/datasource/contract"
	primitive "github.com/viant/forge/backend/provider/contract"
	"github.com/viant/forge/backend/reporting/registry"
	identity "github.com/viant/forge/backend/resource"
	resourcehost "github.com/viant/forge/backend/resourcehost"
	auth "github.com/viant/mcp/client/auth/link"
)

// The two cohort replicas deliberately publish the same report name.
type cohortReportFixture struct {
	schedulerMetadataFixture
	publisher string
	report    primitive.ResourceState
}

func (f *cohortReportFixture) Namespaces(context.Context) ([]primitive.NamespaceCapabilities, error) {
	return []primitive.NamespaceCapabilities{{ProviderIdentity: f.publisher, Namespace: f.namespace, Kinds: []primitive.KindSupport{{Kind: "report", FormatVersions: []int64{1}, Operations: []string{"list", "get"}}}}}, nil
}

func (f *cohortReportFixture) List(_ context.Context, kind, namespace string) ([]primitive.ResourceState, error) {
	if kind == "report" && (namespace == "" || namespace == f.namespace) {
		return []primitive.ResourceState{f.report}, nil
	}
	return nil, nil
}

func (f *cohortReportFixture) Get(_ context.Context, uri string) (*primitive.ResourceState, error) {
	if uri != f.report.URI {
		return nil, identity.ErrResourceDenied
	}
	return &f.report, nil
}

func (f *cohortReportFixture) CurrentHash(ctx context.Context, uri string) (string, error) {
	state, err := f.Get(ctx, uri)
	if err != nil {
		return "", err
	}
	return state.ContentFingerprint, nil
}

func reportCohort(t *testing.T, namespace string) ArtifactCohortBinding {
	t.Helper()
	const spec = `{"version":1,"kind":"reportSpec","source":{"kind":"report","containerId":"sample","stateKey":"sample","dataSourceRef":"rows"},"title":"Summary","scope":{"dataSourceRef":"rows","params":[{"id":"batch","label":"Batch","kind":"text","required":false,"value":"sample-scope"}]},"parameters":{"viewMode":"table","groupBy":"","pageSize":10,"orderField":"","orderDir":"asc"},"layoutIntent":{"kind":"stack","resultPanePosition":"bottom","blockOrder":["details"]},"refinements":[],"calculatedFields":[],"datasets":[{"id":"primary","dataSourceRef":"rows","request":{}}],"blocks":[{"id":"details","kind":"tableBlock","datasetRef":"primary","columns":[{"key":"name","label":"Name"}]}]}`
	descriptor := json.RawMessage(`{"id":"rows","cardinality":"collection","backend":{"kind":"mcp_tool","service":"records","method":"read"}}`)
	definition, err := json.Marshal(registry.ReportEnvelope{SchemaVersion: 1, ReportDocument: json.RawMessage(`{"title":"Summary"}`), ReportSpec: json.RawMessage(strings.ReplaceAll(spec, "sample-scope", namespace)), DataSources: map[string]json.RawMessage{"rows": descriptor}})
	require.NoError(t, err)
	source := &cohortReportFixture{schedulerMetadataFixture: schedulerMetadataFixture{namespace: namespace}, publisher: namespace + "-publisher", report: primitive.ResourceState{
		Kind: "report", Name: "summary", Namespace: namespace, Title: "Summary", URI: "report://" + namespace + "/summary", FormatVersion: 1,
		ContentFingerprint: identity.ContentFingerprint(definition), DefinitionBytes: definition,
	}}
	return ArtifactCohortBinding{ConnectionName: namespace + "-source", ProviderIdentity: source.publisher, Namespaces: []string{namespace}, Source: source, Metadata: source}
}

func TestReportToolsUseNamespaceCohortDefinitions(t *testing.T) {
	for _, key := range []string{"AGENTLY_DB_DRIVER", "AGENTLY_DB_DSN", "AGENTLY_DB_PATH", "AGENTLY_DB_SECRETS"} {
		t.Setenv(key, "")
	}
	root, runtimeRoot := workspace.Root(), workspace.RuntimeRoot()
	t.Cleanup(func() { workspace.SetRoot(root); workspace.SetRuntimeRoot(runtimeRoot) })
	ctx := auth.WithProvider(auth.WithCanonicalUserID(auth.InjectUser(t.Context(), "reader"), "reader"), "local")
	rt, _, _, err := appserver.BuildWorkspaceRuntime(ctx, appserver.RuntimeOptions{WorkspaceRoot: t.TempDir(), SkipRegistryInitialize: true, Defaults: &execconfig.Defaults{Reporting: execconfig.ReportingDefaults{Enabled: true, QueueIntervalMs: 60000}}})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, rt.Close(context.Background())) })
	bindings := []ArtifactCohortBinding{reportCohort(t, "north"), reportCohort(t, "south")}
	_, stop, err := activateArtifactCohorts(ctx, ctx, rt, "", bindings, nil)
	t.Cleanup(stop)
	require.NoError(t, err)
	competitor := reportCohort(t, "north")
	competitor.ConnectionName = "competing-source"
	competitor.ProviderIdentity = "competing-publisher"
	competitor.Metadata.(*cohortReportFixture).publisher = competitor.ProviderIdentity
	_, closeCompetitor, err := activateArtifactCohorts(ctx, ctx, rt, "", []ArtifactCohortBinding{competitor}, nil)
	closeCompetitor()
	require.ErrorIs(t, err, resourcehost.ErrCollision)
	calls := 0
	rt.Reporting.SetResourceDatasetExecutor(func(_ context.Context, pin identity.ResolvedResource, _ *datasource.DataSource, _ map[string]any) (*datasource.FetchResult, error) {
		calls++
		return &datasource.FetchResult{Rows: []map[string]any{{"name": pin.ProviderIdentity}}}, nil
	})
	for _, binding := range bindings {
		namespace := binding.Namespaces[0]
		raw, err := rt.Registry.Execute(ctx, "reporting:list_reports", map[string]any{"namespace": namespace})
		require.NoError(t, err)
		var listed reporting.ListReportsResult
		require.NoError(t, json.Unmarshal([]byte(raw), &listed))
		require.Len(t, listed.Reports, 1)
		for _, selected := range []string{namespace, "different-scope"} {
			scoped, err := rt.Registry.Execute(ctx, "reporting:list_reports", map[string]any{"namespace": namespace, "scope": map[string]any{"key": "batch", "value": selected}})
			require.NoError(t, err)
			var filtered reporting.ListReportsResult
			require.NoError(t, json.Unmarshal([]byte(scoped), &filtered))
			if selected == namespace {
				require.Len(t, filtered.Reports, 1)
			} else {
				require.Empty(t, filtered.Reports)
			}
		}
		_, err = rt.Registry.Execute(ctx, "reporting:list_reports", map[string]any{"namespace": namespace, "unknownFilter": "old-selection"})
		require.Error(t, err, "an unrecognized legacy selector cannot broaden discovery")
		pin := listed.Reports[0].Resource
		require.NotNil(t, pin)
		require.Equal(t, "report://"+namespace+"/summary", pin.URI)
		require.Equal(t, binding.ProviderIdentity, pin.ProviderIdentity)
		raw, err = rt.Registry.Execute(ctx, "reporting:run_report", map[string]any{"resolvedResource": pin})
		require.NoError(t, err)
		var run reporting.ExecuteResourceResult
		require.NoError(t, json.Unmarshal([]byte(raw), &run))
		require.Equal(t, binding.ProviderIdentity, run.Resource.ProviderIdentity)
		require.Contains(t, string(run.ReportFill), binding.ProviderIdentity)
	}
	require.Equal(t, 2, calls)
	_, err = rt.Registry.Execute(ctx, "reporting:run_report", map[string]any{"resource": map[string]any{"uri": "report://north/missing"}})
	require.Error(t, err)
	require.Equal(t, 2, calls)
}
