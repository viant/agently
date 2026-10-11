package agently

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	ds "github.com/viant/agently-core/protocol/datasource"
	identity "github.com/viant/agently-core/protocol/resource"
	"github.com/viant/agently-core/runtime/requestctx"
	"github.com/viant/agently-core/sdk/api"
	datasources "github.com/viant/agently-core/service/datasource"
	forge "github.com/viant/agently-core/service/primitiveprovider"
	"github.com/viant/agently-core/service/ui/permittedview"
	"github.com/viant/authz"
	"github.com/viant/authz/gating"
	"github.com/viant/forge/backend/types"
)

type nativePermissionExecutor func(context.Context, string, map[string]interface{}) (string, error)

func (f nativePermissionExecutor) Execute(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	return f(ctx, name, args)
}

func TestNativeWindowFullSelectionFidelityIncludesOpaqueIDs(t *testing.T) {
	for _, value := range []any{"opaque:item/alpha", json.Number("9007199254740993"), 42} {
		binding := bootstrapDeclaration()
		host := &nativeWindowHost{config: NativeWindowHostConfig{Bootstrap: []WindowBootstrapBinding{binding}, ToolCapabilities: map[string]string{"arbitrary:Patch": "write"}, ExecutionBindings: []NativeWindowExecutionBinding{{URI: binding.URI, DataSource: "change", IdentitySelectors: []string{"Objects.*.Id"}}}}}
		descriptor := &ds.DataSource{ID: "change", Backend: &ds.Backend{Kind: ds.BackendMCPTool, Service: "arbitrary", Method: "Patch"}}
		// This verifies generic body fidelity, not an invented legacy-v1 OPEN
		// resolver for opaque IDs. Complete selection signing is tested separately.
		ctx := requestctx.WithWindowParameters(context.Background(), map[string]any{"Selected": []any{"another-selected", value}})
		if err := host.authorizeWindow(ctx, binding.URI, authoredBootstrapFixture(), descriptor, map[string]any{"Objects": []map[string]any{{"Id": value}}}); err != nil {
			t.Fatal(err)
		}
		if err := host.authorizeWindow(ctx, binding.URI, authoredBootstrapFixture(), descriptor, map[string]any{"Objects": []map[string]any{{"Id": "foreign"}}}); !errors.Is(err, identity.ErrResourceDenied) {
			t.Fatal("foreign selection admitted", err)
		}
	}
}

func TestNativeBusinessAdmissionOccursOnlyAtOriginalWindowOpen(t *testing.T) {
	f := newNativeDeliveryFixture(t)
	f.window.Authorization = &types.AuthorizationSpec{Scope: "principal", ResourceType: "widget", RequestedGlobalCapabilities: []string{"write"}}
	openCalls := 0
	resolver := &permittedview.MCPResolver{ToolName: "window:BusinessOpen", Executor: nativePermissionExecutor(func(context.Context, string, map[string]any) (string, error) {
		openCalls++
		raw, _ := json.Marshal(&permittedview.Snapshot{AuthorizationVersion: "open-business", ExpiresAt: time.Now().Add(time.Minute), GlobalCapabilities: map[string]bool{"write": false}})
		return string(raw), nil
	})}
	f.business = resolver
	admission := &permittedview.OpenAdmission{Runtime: permittedview.NewRuntime(resolver)}
	bridge := forge.NewService(&forge.Config{WindowDefinitions: f.host.catalog, WindowSelectionEmitter: f.host.delivery.emitSelection, WindowOpenAdmission: func(ctx context.Context, pin identity.ResolvedResource, window *types.Window, parameters map[string]any) (*forge.WindowOpenDecision, error) {
		result, err := admission.ApplyDecision(ctx, pin, window, parameters)
		if err != nil {
			return nil, err
		}
		return &forge.WindowOpenDecision{Window: result.Window, ValidUntil: result.ExpiresAt}, nil
	}})
	admitted, err := bridge.AdmitWindowOpenDecision(f.ctx, f.window, f.parameters)
	if err != nil {
		t.Fatal(err)
	}
	emitted, err := bridge.EmitWindowSelection(f.ctx, admitted.Window, f.parameters)
	if err != nil {
		t.Fatal(err)
	}
	dispatches := 0
	service := f.service(t, nativePermissionExecutor(func(context.Context, string, map[string]any) (string, error) {
		dispatches++
		return `{"data":[{"id":42}]}`, nil
	}))
	if _, err = service.Fetch(f.ctx, "rows", nil, fetchNativeOptions(emitted, f.parameters)); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Fetch(f.ctx, "change", map[string]any{"Objects": []map[string]any{{"Id": 42}}}, fetchNativeOptions(emitted, f.parameters)); err != nil {
		t.Fatal(err)
	}
	f.principal.IdentityRevision = "credential-refresh"
	if _, err = service.Fetch(f.ctx, "change", map[string]any{"Objects": []map[string]any{{"Id": 42}}}, fetchNativeOptions(emitted, f.parameters)); err != nil {
		t.Fatal(err)
	}
	if dispatches != 3 || openCalls != 1 {
		t.Fatalf("open business calls=%d managing dispatch=%d err=%v", openCalls, dispatches, err)
	}
}

func fetchNativeOptions(window *types.Window, parameters map[string]any) datasources.FetchOptions {
	return datasources.FetchOptions{Resource: window.Resource, Target: window.ResourceTarget, SelectionBinding: window.SelectionBinding, WindowParameters: parameters}
}

func TestNativeWindowDeclaredParentUsesManagingMCPOnly(t *testing.T) {
	f := newNativeDeliveryFixture(t)
	f.host.config.ExecutionBindings[0].ParentSelectors = map[string]string{"Objects.*.ParentId": "parentKey"}
	source := &authoredBootstrapSource{rows: []map[string]any{{"wireKey": 42, "parentKey": 7}}}
	f.host.datasource = source
	ctx := requestctx.WithWindowParameters(requestctx.WithWindowTarget(requestctx.WithResolvedResource(f.ctx, *f.window.Resource), f.window.ResourceTarget), f.parameters)
	descriptor := &ds.DataSource{ID: "change", Backend: &ds.Backend{Kind: ds.BackendMCPTool, Service: "arbitrary", Method: "Patch"}}
	for _, parent := range []int{7, 999} {
		err := f.host.authorizeWindow(ctx, f.window.Resource.URI, f.window, descriptor, map[string]any{"Objects": []map[string]any{{"Id": 42, "ParentId": parent}}})
		if (err == nil) != (parent == 7) {
			t.Fatalf("parent=%d err=%v", parent, err)
		}
	}
	if source.calls != 2 || source.last.Resource.URI != f.window.Resource.URI || !source.last.Cache.BypassCache || source.last.SelectionBinding != nil {
		t.Fatal("parent read changed original resource/proofs or recursively prepared")
	}
}

func TestNativeAuthorityFailureClassificationIsSanitized(t *testing.T) {
	for _, tc := range []struct {
		name             string
		input, errorWant error
		status           int
	}{
		{"deadline", fmt.Errorf("sensitive payload:%w", context.DeadlineExceeded), context.DeadlineExceeded, 0},
		{"unavailable", fmt.Errorf("sensitive payload:%w", permittedview.ErrUnavailable), permittedview.ErrUnavailable, 0},
		{"authz outage", fmt.Errorf("sensitive payload:%w", authz.ErrUnavailable), permittedview.ErrUnavailable, 0},
		{"gate outage", fmt.Errorf("sensitive payload:%w", gating.ErrUnavailable), permittedview.ErrUnavailable, 0},
		{"unknown", errors.New("sensitive payload unavailable"), identity.ErrResourceDenied, 0},
		{"denial before outage", errors.Join(identity.ErrResourceDenied, permittedview.ErrUnavailable), identity.ErrResourceDenied, 0},
		{"unauthorized", &ds.UpstreamAuthorizationError{Status: 401, Cause: errors.New("sensitive payload")}, nil, 401},
		{"forbidden", &ds.UpstreamAuthorizationError{Status: 403, Cause: errors.New("sensitive payload")}, nil, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := nativeWindowAuthorityError(tc.input)
			if tc.errorWant != nil && !errors.Is(err, tc.errorWant) {
				t.Fatalf("lost classification: %v", err)
			}
			if tc.status != 0 {
				var up *ds.UpstreamAuthorizationError
				if !errors.As(err, &up) || up.Status != tc.status {
					t.Fatal(err)
				}
			}
			if strings.Contains(err.Error(), "sensitive payload") {
				t.Fatal("payload leaked", err)
			}
		})
	}
}

type expiringNativeBootstrap struct {
	deadline time.Time
	calls    int
}

func (s *expiringNativeBootstrap) FetchDatasource(context.Context, *api.FetchDatasourceInput) (*api.FetchDatasourceOutput, error) {
	s.calls++
	time.Sleep(time.Until(s.deadline) + time.Millisecond)
	return &api.FetchDatasourceOutput{Rows: []map[string]any{{"wireKey": 42, "parentKey": 7}}}, nil
}
func TestNativeWindowMutationRejectsResourceLeaseExpiredDuringParentBootstrap(t *testing.T) {
	f := newNativeDeliveryFixture(t)
	f.host.config.ExecutionBindings[0].ParentSelectors = map[string]string{"Objects.*.ParentId": "parentKey"}
	deadline := time.Now().Add(50 * time.Millisecond)
	source := &expiringNativeBootstrap{deadline: deadline}
	f.host.datasource = source
	pin := *f.window.Resource
	pin.ValidUntil = deadline
	ctx := requestctx.WithWindowParameters(requestctx.WithWindowTarget(requestctx.WithResolvedResource(f.ctx, pin), f.window.ResourceTarget), f.parameters)
	descriptor := &ds.DataSource{ID: "change", Backend: &ds.Backend{Kind: ds.BackendMCPTool, Service: "arbitrary", Method: "Patch"}}
	err := f.host.authorizeWindow(ctx, pin.URI, f.window, descriptor, map[string]any{"Objects": []map[string]any{{"Id": 42, "ParentId": 7}}})
	if source.calls != 1 || !errors.Is(err, identity.ErrResourceDenied) {
		t.Fatalf("expired resource lease admitted: calls=%d err=%v", source.calls, err)
	}
}
