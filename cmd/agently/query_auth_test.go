package agently

import (
	"context"
	"github.com/viant/agently-core/sdk"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExplicitRejectedTokenNeverFallsBackToAnotherIdentity(t *testing.T) {
	t.Setenv("AGENTLY_TOKEN", "")
	t.Setenv("AGENTLY_OOB_SECRETS", "invalid-unused-oob-reference")
	for _, mode := range []string{"chat", "tools"} {
		t.Run(mode, func(t *testing.T) {
			fallbackCalls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/api/auth/session" {
					http.Error(w, "rejected", http.StatusUnauthorized)
					return
				}
				fallbackCalls++
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"username":"other-identity"}`))
			}))
			defer server.Close()
			client, err := sdk.NewHTTP(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			providers := []authProviderInfo{{Type: "local", DefaultUsername: "other"}}
			if mode == "chat" {
				err = (&ChatCmd{Token: "invalid-test-token", User: "other"}).ensureAuth(context.Background(), client, providers)
			} else {
				err = ensureToolAuth(context.Background(), client, providers, "invalid-test-token", "existing-session", "", "", "")
			}
			if err == nil {
				t.Fatal("rejected explicit token succeeded")
			}
			if fallbackCalls != 0 {
				t.Fatalf("fallback requests=%d", fallbackCalls)
			}
		})
	}
}
