package agently

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"github.com/viant/agently-core/app/host"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/viant/authz/oauth"
)

func TestHTTPAuthorizationConfigFailsClosedBeforeProviderBinding(t *testing.T) {
	for name, raw := range map[string]string{
		"missing":               "",
		"null":                  "null",
		"unknown field":         `{"kind":"oidc-userinfo","browserAccount":"forged"}`,
		"trailing object":       `{"kind":"oidc-userinfo"} {}`,
		"unregistered provider": `{"kind":"custom-private-provider"}`,
		"missing endpoints":     `{"kind":"oidc-userinfo"}`,
	} {
		t.Run(name, func(t *testing.T) {
			providers, err := host.HTTPProviders(context.Background(), json.RawMessage(raw), nil)
			if err == nil || providers.Identity != nil {
				t.Fatal("invalid operator configuration produced an authority")
			}
		})
	}
}

func TestConfiguredHTTPAuthorizationUsesDeployedEndpointWithoutCustomHost(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var wrongAccount atomic.Bool
	var identityReads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/keys" {
			if r.Header.Get("Authorization") != "" {
				t.Error("bearer sent to JWKS")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]string{
				"kty": "RSA", "kid": "test", "alg": "RS256", "use": "sig",
				"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
				"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
			}}})
			return
		}
		if r.URL.Path != "/identity" {
			http.NotFound(w, r)
			return
		}
		identityReads.Add(1)
		if r.Header.Get("Authorization") == "" {
			t.Error("missing user credential")
		}
		account := "account-2"
		if wrongAccount.Load() {
			account = "different-account"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"sub": "user-1", "account": account, "roles": []string{"reader"}})
	}))
	defer server.Close()
	config := oauth.HTTPIdentityConfig{
		Kind: "oidc-userinfo", Issuer: server.URL, Audience: "workspace", Algorithms: []string{"RS256"},
		UserInfoURL: server.URL + "/identity", JWKSURL: server.URL + "/keys", JWKSRefreshInterval: "1m", FactLease: "30s",
		TenantForAllAccounts: "workspace-tenant",
		Mapping:              oauth.HTTPIdentityMapping{Subject: "sub", AccountID: "account", JWTAccountID: "account", Roles: "roles"},
	}
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	providers, err := host.HTTPProviders(context.Background(), raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{"iss": server.URL, "sub": "user-1", "aud": "workspace", "exp": time.Now().Add(time.Minute).Unix(), "account": "account-2"})
	token.Header["kid"] = "test"
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	ctx := oauth.WithBearer(context.Background(), signed)
	if _, err = providers.Identity.ResolvePrincipal(ctx); err != nil {
		t.Fatal(err)
	}
	if identityReads.Load() == 0 {
		t.Fatal("configured endpoint was not used")
	}
	root := t.TempDir()
	if err = os.WriteFile(filepath.Join(root, "catalog.yaml"), []byte("baseURL: .\nwindows: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	settings, err := json.Marshal(authorizationWorkspaceConfig("authz", "configured-http", "mapped").Raw)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "config.yaml"), settings, 0600); err != nil {
		t.Fatal(err)
	}
	authorizationConfig, err := json.Marshal(hostAuthorizationFile{SchemaVersion: 1, ProviderRef: "configured-http", CapabilityMappingRef: "mapped", PolicyVersion: "revision1", CatalogPath: "catalog.yaml", Identity: raw})
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "authz.json")
	if err = os.WriteFile(configPath, authorizationConfig, 0600); err != nil {
		t.Fatal(err)
	}
	if err = ValidateHostAuthorizationConfiguration(ServeOptions{WorkspacePath: root, AuthzConfigPath: configPath}); err != nil {
		t.Fatalf("ordinary CLI configuration requires no factory: %v", err)
	}
	wrongAccount.Store(true)
	if _, err = providers.Identity.ResolvePrincipal(ctx); err == nil {
		t.Fatal("cross-account identity response accepted")
	}
}
