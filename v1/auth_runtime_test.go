package v1

import (
	"context"
	"testing"

	coreauth "github.com/viant/agently-core/service/auth"
	vauth "github.com/viant/agently/internal/auth"
	scyauth "github.com/viant/scy/auth"
)

func TestWithAuthUserBridgesV1AndCoreContexts(t *testing.T) {
	tokens := &scyauth.Token{}
	tokens.Token.AccessToken = "access-token"

	ctx := withAuthUser(context.Background(), &authUser{
		Subject: "devuser",
		Email:   "devuser@example.com",
		Tokens:  tokens,
	})

	if got := vauth.EffectiveUserID(ctx); got != "devuser" {
		t.Fatalf("v1 auth effective user = %q, want %q", got, "devuser")
	}
	if got := coreauth.EffectiveUserID(ctx); got != "devuser" {
		t.Fatalf("core auth effective user = %q, want %q", got, "devuser")
	}
	if got := vauth.MCPAuthToken(ctx, false); got != "access-token" {
		t.Fatalf("v1 auth MCP token = %q, want %q", got, "access-token")
	}
	if got := coreauth.MCPAuthToken(ctx, false); got != "access-token" {
		t.Fatalf("core auth MCP token = %q, want %q", got, "access-token")
	}
}
