package agently

import (
	"context"
	"testing"

	"github.com/viant/agently-core/app/executor"
	identity "github.com/viant/agently-core/protocol/resource"
	"github.com/viant/agently-core/sdk/api"
	"github.com/viant/agently-core/service/ui/permittedview"
	"github.com/viant/forge/backend/types"
)

type bootstrapBackendFixture struct{}

func (bootstrapBackendFixture) FetchDatasource(context.Context, *api.FetchDatasourceInput) (*api.FetchDatasourceOutput, error) {
	return &api.FetchDatasourceOutput{}, nil
}
func TestWindowBootstrapFactoryBindsReadyRuntimeOnceAndPreservesBuilderHook(t *testing.T) {
	ctx := context.Background()
	configured, calls := 0, 0
	options, bind := prepareWindowBootstrap(ServeOptions{ConfigureBuilder: func(context.Context, *executor.Builder) error { configured++; return nil }, WindowOpenBootstrapFactory: func(_ context.Context, source WindowBootstrapDatasource) (permittedview.OpenBootstrap, permittedview.OpenSelectionCheck, error) {
		if source == nil {
			t.Fatal("factory ran without ready datasource stack")
		}
		calls++
		return func(context.Context, identity.ResolvedResource, *types.Window, map[string]any) (map[string]any, error) {
			return map[string]any{"id": 21}, nil
		}, func(context.Context, *types.Window, map[string]any, map[string]any) error { return nil }, nil
	}})
	if err := options.ConfigureBuilder(ctx, executor.NewBuilder()); err != nil || configured != 1 || calls != 0 {
		t.Fatalf("premature binding or lost builder hook: %d/%d %v", configured, calls, err)
	}
	if err := bind(ctx, nil); err == nil || calls != 0 {
		t.Fatal("unready runtime accepted")
	}
	if err := bind(ctx, bootstrapBackendFixture{}); err != nil || calls != 1 {
		t.Fatalf("runtime binding: %v calls=%d", err, calls)
	}
	if err := bind(ctx, bootstrapBackendFixture{}); err == nil || calls != 1 {
		t.Fatal("factory runtime silently replaced")
	}
}
func TestWindowBootstrapRejectsIncompleteFactory(t *testing.T) {
	_, bind := prepareWindowBootstrap(ServeOptions{WindowOpenBootstrapFactory: func(context.Context, WindowBootstrapDatasource) (permittedview.OpenBootstrap, permittedview.OpenSelectionCheck, error) {
		return nil, nil, nil
	}})
	if err := bind(context.Background(), bootstrapBackendFixture{}); err == nil {
		t.Fatal("incomplete factory granted bootstrap")
	}
}
