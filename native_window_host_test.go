package agently

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	ds "github.com/viant/agently-core/protocol/datasource"
	identity "github.com/viant/agently-core/protocol/resource"
	requestctx "github.com/viant/agently-core/runtime/requestctx"
	"github.com/viant/agently-core/sdk/api"
	dssvc "github.com/viant/agently-core/service/datasource"
	"github.com/viant/agently-core/service/ui/permittedview"
	"github.com/viant/authz"
	"github.com/viant/authz/gating"
	"github.com/viant/forge/backend/types"
	"reflect"
	"strings"
	"testing"
	"time"
)

type nativePermissionExecutor func(context.Context, string, map[string]interface{}) (string, error)

func (f nativePermissionExecutor) Execute(c context.Context, n string, a map[string]interface{}) (string, error) {
	return f(c, n, a)
}
func TestNativeWindowLegacyPermissionAndSelectionBeforeExecution(t *testing.T) {
	for _, tc := range []struct {
		name      string
		allow     bool
		selected  any
		mutation  any
		method    string
		wantCalls int
	}{
		{"allow", true, []int{42}, 42, "Patch", 1},
		{"legacy deny", false, []int{42}, 42, "Patch", 0},
		{"foreign mutation", true, []int{42}, 99, "Patch", 0},
		{"missing selector", true, nil, 42, "Patch", 0},
		{"unmapped tool", true, []int{42}, 42, "Other", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := authoredBootstrapFixture()
			binding := bootstrapDeclaration()
			d := &ds.DataSource{ID: "change", Backend: &ds.Backend{Kind: ds.BackendMCPTool, Service: "native", Method: tc.method}}
			permissions := &permittedview.MCPResolver{ToolName: "steward:ResourceAuthorization", Executor: nativePermissionExecutor(func(_ context.Context, n string, args map[string]interface{}) (string, error) {
				if n != "steward:ResourceAuthorization" || args["ResourceType"] != "widget" {
					t.Fatal("legacy permission binding replaced")
				}
				ids := args["ResourceIDs"].([]int)
				if len(ids) != 1 || ids[0] != 42 {
					t.Fatal("browser changed authorization ID", ids)
				}
				raw, _ := json.Marshal(&permittedview.Snapshot{AuthorizationVersion: "real", ExpiresAt: time.Now().Add(time.Minute), Resources: map[string]*permittedview.Resource{"42": {ID: 42, Type: "widget", Capabilities: map[string]bool{"read": true, "write": tc.allow}}}})
				return string(raw), nil
			})}
			h := &nativeWindowHost{permissions: permissions, config: NativeWindowHostConfig{Bootstrap: []WindowBootstrapBinding{binding}, ToolCapabilities: map[string]string{"native:Patch": "write"}, ExecutionBindings: []NativeWindowExecutionBinding{{URI: binding.URI, DataSource: "change", IdentitySelectors: []string{"Objects.*.Id"}}}}}
			calls := 0
			store := dssvc.NewMemoryStore()
			store.Put(d)
			service := dssvc.New(dssvc.Options{Store: store, AuthorizeDefinition: func(ctx context.Context, d *ds.DataSource, in map[string]interface{}) error {
				return h.authorizeWindow(ctx, binding.URI, w, d, in)
			}, Executor: nativePermissionExecutor(func(context.Context, string, map[string]interface{}) (string, error) {
				calls++
				return `{"data":[{"id":42}]}`, nil
			})})
			ctx := requestctx.WithWindowParameters(context.Background(), map[string]any{"Selected": tc.selected})
			// Fetch transports parameters as request hints; no permission facts.
			_, err := service.Fetch(ctx, "change", map[string]interface{}{"Objects": []map[string]interface{}{{"Id": tc.mutation}}}, dssvc.FetchOptions{WindowParameters: map[string]any{"Selected": tc.selected}})
			if calls != tc.wantCalls {
				t.Fatalf("executions=%d want=%d error=%v", calls, tc.wantCalls, err)
			}
			if tc.wantCalls == 0 && !errors.Is(err, identity.ErrResourceDenied) {
				t.Fatal("denial did not fail closed", err)
			}
			if tc.wantCalls > 0 && err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestNativeWindowPrincipalReadPreservesOriginalGlobalSemantics(t *testing.T) {
	h := &nativeWindowHost{config: NativeWindowHostConfig{ToolCapabilities: map[string]string{"native:List": "read"}}, permissions: &permittedview.MCPResolver{ToolName: "original", Executor: nativePermissionExecutor(func(context.Context, string, map[string]interface{}) (string, error) {
		raw, _ := json.Marshal(&permittedview.Snapshot{AuthorizationVersion: "original", ExpiresAt: time.Now().Add(time.Minute), GlobalCapabilities: map[string]bool{"create": false, "bulkEdit": false}})
		return string(raw), nil
	})}}
	w := &types.Window{Authorization: &types.AuthorizationSpec{Scope: "principal", ResourceType: "advertiser", RequestedGlobalCapabilities: []string{"create", "bulkEdit"}}}
	if err := h.authorizeWindow(context.Background(), "window://tenant/list", w, &ds.DataSource{ID: "list", Backend: &ds.Backend{Kind: ds.BackendMCPTool, Service: "native", Method: "List"}}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestNativeWindowMutationParentUsesFreshNativeBootstrap(t *testing.T) {
	binding := bootstrapDeclaration()
	w := authoredBootstrapFixture()
	source := &authoredBootstrapSource{rows: []map[string]any{{"wireKey": 42, "parentKey": 7}}}
	h := &nativeWindowHost{datasource: source, permissions: &permittedview.MCPResolver{ToolName: "original", Executor: nativePermissionExecutor(func(context.Context, string, map[string]interface{}) (string, error) {
		raw, _ := json.Marshal(&permittedview.Snapshot{AuthorizationVersion: "actual", ExpiresAt: time.Now().Add(time.Minute), Resources: map[string]*permittedview.Resource{"42": {ID: 42, Type: "widget", Capabilities: map[string]bool{"read": true, "write": true}}}})
		return string(raw), nil
	})}, config: NativeWindowHostConfig{Bootstrap: []WindowBootstrapBinding{binding}, ToolCapabilities: map[string]string{"native:Patch": "write"}, ExecutionBindings: []NativeWindowExecutionBinding{{URI: binding.URI, DataSource: "change", IdentitySelectors: []string{"Objects.*.Id"}, ParentSelectors: map[string]string{"Objects.*.ParentId": "parentKey"}}}}}
	pin := identity.ResolvedResource{URI: binding.URI}
	target := &types.WindowTarget{Platform: "web"}
	ctx := requestctx.WithWindowParameters(requestctx.WithWindowTarget(requestctx.WithResolvedResource(context.Background(), pin), target), map[string]any{"Selected": []int{42}, "ParentSelection": []int{999}})
	descriptor := &ds.DataSource{ID: "change", Backend: &ds.Backend{Kind: ds.BackendMCPTool, Service: "native", Method: "Patch"}}
	for _, parent := range []int{7, 999} {
		err := h.authorizeWindow(ctx, binding.URI, w, descriptor, map[string]interface{}{"Objects": []map[string]interface{}{{"Id": 42, "ParentId": parent}}})
		if (err == nil) != (parent == 7) {
			t.Fatalf("parent=%d err=%v", parent, err)
		}
	}
	if source.calls != 2 || source.last.Resource.URI != binding.URI || !reflect.DeepEqual(source.last.Target, target) || !source.last.Cache.BypassCache {
		t.Fatal("parent read did not retain exact native bootstrap", source.calls)
	}
}

func TestNativeScopedRecoveryChecksFreshLegacyRootRead(t *testing.T) {
	calls := 0
	allow := true
	binding := bootstrapDeclaration()
	window := authoredBootstrapFixture()
	host := &nativeWindowHost{config: NativeWindowHostConfig{Bootstrap: []WindowBootstrapBinding{binding}, ToolCapabilities: map[string]string{"native:Rows": "read"}}, permissions: &permittedview.MCPResolver{ToolName: "original:ResourceAuthorization", Executor: nativePermissionExecutor(func(_ context.Context, name string, in map[string]interface{}) (string, error) {
		calls++
		if name != "original:ResourceAuthorization" || in["ResourceType"] != "widget" {
			t.Fatal("root authority replaced")
		}
		raw, _ := json.Marshal(&permittedview.Snapshot{AuthorizationVersion: "actual", ExpiresAt: time.Now().Add(time.Minute), Resources: map[string]*permittedview.Resource{"42": {ID: 42, Type: "widget", Capabilities: map[string]bool{"read": allow}}}})
		return string(raw), nil
	})}}
	descriptor := &ds.DataSource{ID: "rows", Backend: &ds.Backend{Kind: ds.BackendMCPTool, Service: "native", Method: "Rows"}}
	ctx := requestctx.WithWindowParameters(context.Background(), map[string]any{"Selected": []int{42}})
	if err := host.authorizeWindowMode(ctx, binding.URI, window, descriptor, nil, true); err != nil {
		t.Fatal(err)
	}
	allow = false
	if err := host.authorizeWindowMode(ctx, binding.URI, window, descriptor, nil, true); !errors.Is(err, identity.ErrResourceDenied) {
		t.Fatal("revocation ignored", err)
	}
	if calls != 2 {
		t.Fatal("cached root authority reused", calls)
	}
}

// Simulate a real bootstrap read that completes after the granted lease ends.
type expiringNativeBootstrap struct {
	deadline time.Time
	calls    int
}

func (s *expiringNativeBootstrap) FetchDatasource(context.Context, *api.FetchDatasourceInput) (*api.FetchDatasourceOutput, error) {
	s.calls++
	time.Sleep(time.Until(s.deadline) + time.Millisecond)
	return &api.FetchDatasourceOutput{Rows: []map[string]any{{"wireKey": 42, "parentKey": 7}}}, nil
}
func TestNativeWindowMutationRejectsLeaseExpiredDuringParentBootstrap(t *testing.T) {
	binding := bootstrapDeclaration()
	source := &expiringNativeBootstrap{}
	host := &nativeWindowHost{datasource: source, permissions: &permittedview.MCPResolver{ToolName: "original", Executor: nativePermissionExecutor(func(context.Context, string, map[string]interface{}) (string, error) {
		source.deadline = time.Now().Add(100 * time.Millisecond)
		raw, _ := json.Marshal(&permittedview.Snapshot{AuthorizationVersion: "actual", ExpiresAt: source.deadline, Resources: map[string]*permittedview.Resource{"42": {ID: 42, Type: "widget", Capabilities: map[string]bool{"read": true, "write": true}}}})
		return string(raw), nil
	})}, config: NativeWindowHostConfig{Bootstrap: []WindowBootstrapBinding{binding}, ToolCapabilities: map[string]string{"native:Patch": "write"}, ExecutionBindings: []NativeWindowExecutionBinding{{URI: binding.URI, DataSource: "change", IdentitySelectors: []string{"Objects.*.Id"}, ParentSelectors: map[string]string{"Objects.*.ParentId": "parentKey"}}}}}
	ctx := requestctx.WithWindowParameters(requestctx.WithWindowTarget(requestctx.WithResolvedResource(context.Background(), identity.ResolvedResource{URI: binding.URI}), &types.WindowTarget{Platform: "web"}), map[string]any{"Selected": []int{42}})
	descriptor := &ds.DataSource{ID: "change", Backend: &ds.Backend{Kind: ds.BackendMCPTool, Service: "native", Method: "Patch"}}
	err := host.authorizeWindow(ctx, binding.URI, authoredBootstrapFixture(), descriptor, map[string]interface{}{"Objects": []map[string]interface{}{{"Id": 42, "ParentId": 7}}})
	if source.calls != 1 {
		t.Fatalf("expected fresh parent read, got %d", source.calls)
	}
	if !errors.Is(err, identity.ErrResourceDenied) {
		t.Fatalf("expired mutation lease admitted: %v", err)
	}
}

func TestNativeWindowAuthorityFailuresRemainTypedAndNeverDispatch(t *testing.T) {
	for _, stage := range []string{"permission", "parent"} {
		for _, tc := range []struct {
			name   string
			err    error
			want   error
			status int
		}{
			{"deadline", fmt.Errorf("sensitive upstream payload: %w", context.DeadlineExceeded), context.DeadlineExceeded, 0},
			{"unavailable", fmt.Errorf("sensitive upstream payload: %w", permittedview.ErrUnavailable), permittedview.ErrUnavailable, 0},
			{"generic authz unavailable", fmt.Errorf("sensitive upstream payload: %w", authz.ErrUnavailable), permittedview.ErrUnavailable, 0},
			{"generic gate unavailable", fmt.Errorf("sensitive upstream payload: %w", gating.ErrUnavailable), permittedview.ErrUnavailable, 0},
			{"canceled", fmt.Errorf("sensitive upstream payload: %w", context.Canceled), context.Canceled, 0},
			{"unauthorized", &ds.UpstreamAuthorizationError{Status: 401, Cause: errors.New("sensitive upstream payload")}, nil, 401},
			{"forbidden", &ds.UpstreamAuthorizationError{Status: 403, Cause: errors.New("sensitive upstream payload")}, nil, 403},
			{"unknown verification failure", errors.New("dependency unavailable sensitive upstream payload"), identity.ErrResourceDenied, 0},
			{"real denial", identity.ErrResourceDenied, identity.ErrResourceDenied, 0},
			{"denial dominates unavailable", errors.Join(identity.ErrResourceDenied, permittedview.ErrUnavailable), identity.ErrResourceDenied, 0},
		} {
			t.Run(stage+"/"+tc.name, func(t *testing.T) {
				binding := bootstrapDeclaration()
				source := &authoredBootstrapSource{rows: []map[string]any{{"wireKey": 42, "parentKey": 7}}}
				permissionCalls := 0
				permissions := &permittedview.MCPResolver{ToolName: "original", Executor: nativePermissionExecutor(func(context.Context, string, map[string]interface{}) (string, error) {
					permissionCalls++
					if stage == "permission" {
						return "", tc.err
					}
					raw, _ := json.Marshal(&permittedview.Snapshot{AuthorizationVersion: "fresh", ExpiresAt: time.Now().Add(time.Minute), Resources: map[string]*permittedview.Resource{"42": {ID: 42, Type: "widget", Capabilities: map[string]bool{"read": true, "write": true}}}})
					return string(raw), nil
				})}
				if stage == "parent" {
					source.err = tc.err
				}
				host := &nativeWindowHost{permissions: permissions, datasource: source, config: NativeWindowHostConfig{Bootstrap: []WindowBootstrapBinding{binding}, ToolCapabilities: map[string]string{"native:Patch": "write"}, ExecutionBindings: []NativeWindowExecutionBinding{{URI: binding.URI, DataSource: "change", IdentitySelectors: []string{"Objects.*.Id"}, ParentSelectors: map[string]string{"Objects.*.ParentId": "parentKey"}}}}}
				descriptor := &ds.DataSource{ID: "change", Backend: &ds.Backend{Kind: ds.BackendMCPTool, Service: "native", Method: "Patch"}}
				store := dssvc.NewMemoryStore()
				store.Put(descriptor)
				dispatches := 0
				service := dssvc.New(dssvc.Options{Store: store, AuthorizeDefinition: func(ctx context.Context, d *ds.DataSource, in map[string]interface{}) error {
					return host.authorizeWindow(requestctx.WithWindowTarget(requestctx.WithResolvedResource(ctx, identity.ResolvedResource{URI: binding.URI}), &types.WindowTarget{Platform: "web"}), binding.URI, authoredBootstrapFixture(), d, in)
				}, Executor: nativePermissionExecutor(func(context.Context, string, map[string]interface{}) (string, error) { dispatches++; return "{}", nil })})
				ctx := requestctx.WithWindowParameters(context.Background(), map[string]any{"Selected": []int{42}})
				for attempt := 0; attempt < 2; attempt++ {
					_, err := service.Fetch(ctx, "change", map[string]interface{}{"Objects": []map[string]interface{}{{"Id": 42, "ParentId": 7}}}, dssvc.FetchOptions{WindowParameters: map[string]any{"Selected": []int{42}}})
					if tc.want != nil && !errors.Is(err, tc.want) {
						t.Fatalf("want typed %v, got %v", tc.want, err)
					}
					if tc.status != 0 {
						var upstream *ds.UpstreamAuthorizationError
						if !errors.As(err, &upstream) || upstream.Status != tc.status {
							t.Fatalf("lost trusted auth status%d: %v", tc.status, err)
						}
					}
					if err == nil || strings.Contains(err.Error(), "sensitive upstream payload") {
						t.Fatalf("verification failure leaked/admitted: %v", err)
					}
				}
				if dispatches != 0 || permissionCalls != 2 {
					t.Fatalf("dispatches=%d fresh permissions=%d", dispatches, permissionCalls)
				}
			})
		}
	}
}
