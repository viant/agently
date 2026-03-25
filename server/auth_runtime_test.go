package server

import (
	"context"
	"net/http/httptest"
	"testing"

	svcauth "github.com/viant/agently-core/service/auth"
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

	if got := svcauth.EffectiveUserID(ctx); got != "devuser" {
		t.Fatalf("auth effective user = %q, want %q", got, "devuser")
	}
	if got := svcauth.MCPAuthToken(ctx, false); got != "access-token" {
		t.Fatalf("auth MCP token = %q, want %q", got, "access-token")
	}
}

func TestEnsureDefaultUser_OAuthBFFDoesNotFallbackToDefaultUsername(t *testing.T) {
	ar := &authRuntime{
		cfg: &authConfig{
			Enabled:         true,
			DefaultUsername: "devuser",
			CookieName:      "agently_session",
			Local:           &localConfig{Enabled: false},
			OAuth:           &oauthConfig{Mode: "bff"},
		},
		sessions: svcauth.NewManager(0, nil),
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/v1/conversations", nil)
	got := ar.ensureDefaultUser(rec, req)
	if got != nil {
		t.Fatalf("expected no default user in oauth bff mode, got %#v", got)
	}
}
