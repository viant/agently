package agently

import (
	"context"
	"testing"

	"github.com/viant/authz/gating"
)

type componentScopeKey string
type componentScopeFixture struct{}

func (componentScopeFixture) BeginMetadataRead(ctx context.Context) (context.Context, func() error, error) {
	return context.WithValue(ctx, componentScopeKey("metadata"), true), func() error { return nil }, nil
}
func (componentScopeFixture) WithoutMetadataRead(ctx context.Context) context.Context {
	return context.WithValue(ctx, componentScopeKey("metadata"), false)
}
func (componentScopeFixture) BeginDecision(ctx context.Context) (context.Context, func() error, error) {
	return context.WithValue(ctx, componentScopeKey("decision"), true), func() error { return nil }, nil
}
func (componentScopeFixture) WithoutDecision(ctx context.Context) context.Context {
	return context.WithValue(ctx, componentScopeKey("decision"), false)
}
func (componentScopeFixture) MetadataReadActive(ctx context.Context) bool {
	return ctx.Value(componentScopeKey("metadata")) == true
}

func TestComponentAuthorityExplicitlyBypassesBothPureCheckScopes(t *testing.T) {
	scope := componentScopeFixture{}
	ctx, _, _ := scope.BeginMetadataRead(context.Background())
	ctx, _, _ = scope.BeginDecision(ctx)
	calls := 0
	fresh := freshComponentAuthority(func(actual context.Context) (gating.Principal, error) {
		calls++
		if actual.Value(componentScopeKey("metadata")) != false || actual.Value(componentScopeKey("decision")) != false {
			t.Fatal("execution reused pure-check scope")
		}
		return gating.Principal{AccountID: "verified"}, nil
	}, scope, scope)
	for i := 0; i < 2; i++ {
		if value, err := fresh(ctx); err != nil || value.AccountID != "verified" {
			t.Fatal(err)
		}
	}
	if calls != 2 || !scope.MetadataReadActive(ctx) || ctx.Value(componentScopeKey("decision")) != true {
		t.Fatal("fresh execution mutated caller scopes or cached authority")
	}
}
