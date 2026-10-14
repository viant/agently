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

func TestNativeWindowProjectionDoesNotAuthorizeDataDispatch(t *testing.T) {
	f := newNativeDeliveryFixture(t)
	f.window.Authorization = &types.AuthorizationSpec{Scope: "principal", ResourceType: "widget", RequestedGlobalCapabilities: []string{"write"}}
	openCalls := 0
	resolver := &permittedview.MCPResolver{ToolName: "window:Principal", Executor: nativePermissionExecutor(func(context.Context, string, map[string]any) (string, error) {
		openCalls++
		raw, _ := json.Marshal(&permittedview.Snapshot{AuthorizationVersion: "open-business", ExpiresAt: time.Now().Add(time.Minute), GlobalCapabilities: map[string]bool{"write": false}})
		return string(raw), nil
	})}
	f.business = resolver
	projection := permittedview.NewRuntime(resolver)
	bridge := forge.NewService(&forge.Config{WindowDefinitions: f.host.catalog, WindowSelectionEmitter: f.host.delivery.emitSelection, WindowUIProjection: func(ctx context.Context, input *permittedview.BoundView) (*types.Window, error) {
		result, err := projection.Project(ctx, input.Window, input)
		if err != nil {
			return nil, err
		}
		return result.Window, nil
	}})
	projected, err := bridge.ProjectWindow(f.ctx, f.window)
	if err != nil {
		t.Fatal(err)
	}
	emitted, err := bridge.EmitWindowSelection(f.ctx, projected, f.parameters)
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
	if dispatches != 3 || openCalls != 0 {
		t.Fatalf("unused UI fact calls=%d managing dispatch=%d err=%v", openCalls, dispatches, err)
	}
}

func fetchNativeOptions(window *types.Window, parameters map[string]any) datasources.FetchOptions {
	return datasources.FetchOptions{Resource: window.Resource, Target: window.ResourceTarget, SelectionBinding: window.SelectionBinding, WindowParameters: parameters}
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
