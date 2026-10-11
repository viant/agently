package agently

import (
	"context"
	"errors"
	identity "github.com/viant/agently-core/protocol/resource"
	"github.com/viant/agently-core/sdk/api"
	"github.com/viant/forge/backend/types"
	"testing"
	"time"
)

type authoredBootstrapSource struct {
	calls int
	last  *api.FetchDatasourceInput
	rows  []map[string]any
	err   error
}

func (s *authoredBootstrapSource) FetchDatasource(_ context.Context, in *api.FetchDatasourceInput) (*api.FetchDatasourceOutput, error) {
	s.calls++
	s.last = in
	if s.err != nil {
		return nil, s.err
	}
	return &api.FetchDatasourceOutput{Rows: s.rows}, nil
}
func authoredBootstrapFixture() *types.Window {
	return &types.Window{WindowKey: "nested/arbitrary", Parameters: []map[string]interface{}{{"name": "Selected", "type": "int[]"}}, Authorization: &types.AuthorizationSpec{Scope: "resource", Resource: &types.AuthorizationResourceSpec{Type: "widget", ID: types.AuthorizationValueSelector{Source: "resource", Selector: "Selected.0"}}}, Schemas: map[string]types.ResourceSchema{"entity": {Identity: []string{"key"}}}, ResourceModels: map[string]types.ResourceModel{"entity": {SchemaRef: "entity", Read: &types.ResourceReadBinding{DataSourceRef: "details"}, Fields: map[string]types.ResourceFieldBinding{"key": {Read: "wireKey"}}}}, DataSource: map[string]types.DataSource{"details": {UniqueKey: []*types.UniqueKey{{Field: "wireKey"}}, Parameters: []types.Parameter{{Name: "Object", In: "windowForm", Location: "Selected"}, {Name: "Parent", In: "windowForm", Location: "ParentSelection"}}}}}
}
func TestConfiguredWindowBootstrapDeclaredIdentityAndProtectedFetch(t *testing.T) {
	source := &authoredBootstrapSource{rows: []map[string]any{{"wireKey": int64(42)}}}
	load, match, err := ConfiguredWindowBootstrap([]WindowBootstrapBinding{bootstrapDeclaration()})(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	w := authoredBootstrapFixture()
	w.ResourceTarget = &types.WindowTarget{Platform: "web"}
	pin := identity.ResolvedResource{URI: "window://tenant/nested/arbitrary", ValidUntil: time.Now().Add(time.Minute)}
	args := map[string]any{"Selected": []int64{42}, "ParentSelection": []int64{7}}
	row, err := load(context.Background(), pin, w, args)
	if err != nil {
		t.Fatal(err)
	}
	if source.last.ID != "details" || source.last.Resource.URI != pin.URI || source.last.Target != w.ResourceTarget || source.last.Cache == nil || !source.last.Cache.BypassCache {
		t.Fatal("protected fetch lost original bindings")
	}
	if err = match(context.Background(), w, args, row); err != nil {
		t.Fatal(err)
	}
	source.err = identity.ErrResourceDenied
	if result, err := load(context.Background(), pin, w, args); !errors.Is(err, identity.ErrResourceDenied) || result != nil {
		t.Fatal("fresh datasource denial bypassed")
	}
	if source.calls != 2 {
		t.Fatal("bootstrap cached a prior result")
	}
	source.err = nil
	source.rows = []map[string]any{{"wireKey": int64(99)}}
	if result, err := load(context.Background(), pin, w, args); !errors.Is(err, identity.ErrResourceDenied) || result != nil {
		t.Fatal("wrong selected row accepted")
	}
	if err = match(context.Background(), w, args, map[string]any{"Selected": []int64{99}}); err == nil {
		t.Fatal("selected identity mismatch accepted")
	}
}
func TestConfiguredWindowBootstrapMissingAndAmbiguousDeclarationsDeny(t *testing.T) {
	source := &authoredBootstrapSource{rows: []map[string]any{{"wireKey": int64(42)}}}
	load, _, _ := ConfiguredWindowBootstrap([]WindowBootstrapBinding{bootstrapDeclaration()})(context.Background(), source)
	cases := []func(*types.Window){
		func(w *types.Window) { w.DataSource = nil },
		func(w *types.Window) { w.Parameters = []map[string]interface{}{{"name": "Selected", "type": "string"}} },
		func(w *types.Window) { w.Authorization.Resource.ID.Selector = "Selected" },
		func(w *types.Window) { ds := w.DataSource["details"]; ds.UniqueKey = nil; w.DataSource["details"] = ds },
		func(w *types.Window) {
			ds := w.DataSource["details"]
			ds.Parameters = append(ds.Parameters, ds.Parameters[0])
			w.DataSource["details"] = ds
		},
	}

	for _, change := range cases {
		w := authoredBootstrapFixture()
		change(w)
		if _, err := load(context.Background(), identity.ResolvedResource{URI: "window://tenant/nested/arbitrary"}, w, map[string]any{"Selected": []int64{42}}); !errors.Is(err, identity.ErrResourceDenied) {
			t.Fatal("undeclared or ambiguous bootstrap accepted")
		}
	}
	if source.calls != 0 {
		t.Fatal("invalid declarations reached data execution")
	}
	if _, _, err := ConfiguredWindowBootstrap(nil)(context.Background(), nil); err == nil {
		t.Fatal("nil protected source accepted")
	}
}
func TestWindowBootstrapDefaultIsLimitedToConfiguredInternalProvider(t *testing.T) {
	legacy, bind := prepareWindowBootstrap(ServeOptions{})
	if legacy.ConfigureBuilder != nil || legacy.WindowOpenBootstrapFactory != nil {
		t.Fatal("legacy runtime changed")
	}
	if err := bind(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	configured, bind := prepareWindowBootstrap(ServeOptions{InternalResourceProviderIdentity: "host/provider"})
	if configured.WindowOpenBootstrapFactory == nil || configured.ConfigureBuilder == nil {
		t.Fatal("configured provider has no generic bootstrap")
	}
	if err := bind(context.Background(), bootstrapBackendFixture{}); err != nil {
		t.Fatal(err)
	}
}

func bootstrapDeclaration() WindowBootstrapBinding {
	return WindowBootstrapBinding{URI: "window://tenant/nested/arbitrary", WindowKey: "nested/arbitrary", ResourceType: "widget", Parameter: "Selected", DataSource: "details", Input: "Object", RowIdentity: "wireKey", IdentityType: "integer"}
}
func TestConfiguredWindowBootstrapValidationAndRequiredSelector(t *testing.T) {
	binding := bootstrapDeclaration()
	if err := validateWindowBootstrapBindings([]WindowBootstrapBinding{binding, binding}); err == nil {
		t.Fatal("duplicate accepted")
	}
	for _, kind := range []string{"", "string", "number"} {
		b := binding
		b.IdentityType = kind
		if err := validateWindowBootstrapBindings([]WindowBootstrapBinding{b}); err == nil {
			t.Fatal("unsupported identity type accepted")
		}
	}
	source := &authoredBootstrapSource{rows: []map[string]any{{"wireKey": int64(42)}}}
	load, _, _ := ConfiguredWindowBootstrap([]WindowBootstrapBinding{binding})(context.Background(), source)
	w := authoredBootstrapFixture()
	ds := w.DataSource["details"]
	required := true
	ds.Parameters[1].Location = "parent.ids"
	ds.Parameters[1].Required = &required
	w.DataSource["details"] = ds
	pin := identity.ResolvedResource{URI: binding.URI}
	if _, err := load(context.Background(), pin, w, map[string]any{"Selected": []int64{42}}); err == nil || source.calls != 0 {
		t.Fatal("missing required selector executed")
	}
	if _, err := load(context.Background(), pin, w, map[string]any{"Selected": []int64{42}, "parent": map[string]any{"ids": []int64{7}}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := source.last.Inputs["Parent"].([]int64); !ok {
		t.Fatal("declared nested selector lost")
	}
	for _, value := range []any{"42", -1, 1.2, float64(9007199254740992)} {
		if _, err := load(context.Background(), pin, w, map[string]any{"Selected": value, "parent": map[string]any{"ids": []int64{7}}}); err == nil {
			t.Fatal("unsupported or imprecise integer accepted")
		}
	}
}

func TestConfiguredBootstrapValidatesExistingAuxiliaryIntegerDeclaration(t *testing.T) {
	source := &authoredBootstrapSource{rows: []map[string]any{{"wireKey": int64(42)}}}
	load, _, _ := ConfiguredWindowBootstrap([]WindowBootstrapBinding{bootstrapDeclaration()})(context.Background(), source)
	w := authoredBootstrapFixture()
	w.Parameters = []map[string]interface{}{{"name": "Selected", "type": "int[]"}, {"name": "ParentSelection", "type": "int[]"}}
	pin := identity.ResolvedResource{URI: bootstrapDeclaration().URI}
	for _, parent := range []any{[]int{-1}, []int{0}, []any{7, "8"}, 1.5} {
		if _, err := load(context.Background(), pin, w, map[string]any{"Selected": []int{42}, "ParentSelection": parent}); err == nil {
			t.Fatal("malformed declared auxiliary identity executed")
		}
	}
	if source.calls != 0 {
		t.Fatal("malformed parent reached protected dispatch")
	}
	if _, err := load(context.Background(), pin, w, map[string]any{"Selected": []int{42}, "ParentSelection": []int{7}}); err != nil {
		t.Fatal(err)
	}
	if values, ok := source.last.Inputs["Parent"].([]int64); !ok || len(values) != 1 || values[0] != 7 {
		t.Fatal("authored integer array was not normalized")
	}
}
