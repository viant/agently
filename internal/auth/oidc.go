package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/viant/scy/auth/authorizer"
	vcfg "github.com/viant/scy/auth/jwt/verifier"
)

// NewOIDCVerifierFromEnv constructs a JWT verifier using JWKS URL from env:
//
//	AGENTLY_OIDC_JWKS_URL
//
// Returns nil when not configured.
func NewOIDCVerifierFromEnv(ctx context.Context) (*vcfg.Service, error) {
	jwks := os.Getenv("AGENTLY_OIDC_JWKS_URL")
	if jwks == "" {
		return nil, nil
	}
	cfg := &vcfg.Config{CertURL: jwks}
	v := vcfg.New(cfg)
	if err := v.Init(ctx); err != nil {
		return nil, err
	}
	return v, nil
}

// JWKSFromBFFConfig derives a JWKS URL from an OAuth client config loaded via scy's authorizer.
// Strategy:
// 1) Build discovery URL candidates from AuthURL/TokenURL path patterns and fetch jwks_uri.
// 2) Fallback to <scheme>://<host>/.well-known/openid-configuration
// 3) Fallback to <scheme>://<host>/.well-known/jwks.json
func JWKSFromBFFConfig(ctx context.Context, configURL string) (string, error) {
	if strings.TrimSpace(configURL) == "" {
		return "", nil
	}
	svc := authorizer.New()
	oc := &authorizer.OAuthConfig{ConfigURL: configURL}
	if err := svc.EnsureConfig(ctx, oc); err != nil {
		return "", err
	}
	if oc.Config == nil {
		return "", nil
	}
	candidates := make([]string, 0, 4)
	// Helper to derive discovery from an auth URL path
	deriveDiscovery := func(raw string) string {
		u, err := url.Parse(raw)
		if err != nil || u == nil || u.Host == "" {
			return ""
		}
		p := u.Path
		// Okta: /oauth2/<as>/v1/authorize -> /oauth2/<as>/.well-known/openid-configuration
		if strings.Contains(p, "/oauth2/") {
			parts := strings.Split(p, "/")
			for i := 0; i < len(parts)-1; i++ {
				if parts[i] == "oauth2" && i+1 < len(parts) {
					base := "/oauth2/" + parts[i+1]
					return u.Scheme + "://" + u.Host + path.Join(base, "/.well-known/openid-configuration")
				}
			}
		}
		// Keycloak: /realms/<realm>/protocol/openid-connect/auth -> /realms/<realm>/.well-known/openid-configuration
		if strings.Contains(p, "/realms/") {
			parts := strings.Split(p, "/")
			for i := 0; i < len(parts)-1; i++ {
				if parts[i] == "realms" && i+1 < len(parts) {
					base := "/realms/" + parts[i+1]
					return u.Scheme + "://" + u.Host + path.Join(base, "/.well-known/openid-configuration")
				}
			}
		}
		return u.Scheme + "://" + u.Host + "/.well-known/openid-configuration"
	}
	if oc.Config.Endpoint.AuthURL != "" {
		if d := deriveDiscovery(oc.Config.Endpoint.AuthURL); d != "" {
			candidates = append(candidates, d)
		}
	}
	if oc.Config.Endpoint.TokenURL != "" {
		if d := deriveDiscovery(oc.Config.Endpoint.TokenURL); d != "" {
			candidates = append(candidates, d)
		}
	}
	// Always try host-level discovery
	if oc.Config.Endpoint.AuthURL != "" {
		if u, err := url.Parse(oc.Config.Endpoint.AuthURL); err == nil && u != nil && u.Host != "" {
			candidates = append(candidates, u.Scheme+"://"+u.Host+"/.well-known/openid-configuration")
			candidates = append(candidates, u.Scheme+"://"+u.Host+"/.well-known/jwks.json")
		}
	}
	// Try discovery first
	for _, d := range candidates {
		if strings.HasSuffix(d, "/.well-known/jwks.json") {
			continue
		}
		if jwks, err := JWKSFromDiscovery(ctx, d); err == nil && strings.TrimSpace(jwks) != "" {
			return jwks, nil
		}
	}
	// Fallback: jwks.json
	for _, c := range candidates {
		if strings.HasSuffix(c, "/.well-known/jwks.json") {
			return c, nil
		}
	}
	return "", nil
}

// JWKSFromDiscovery fetches the OpenID discovery document and returns jwks_uri.
func JWKSFromDiscovery(ctx context.Context, discoveryURL string) (string, error) {
	u := strings.TrimSpace(discoveryURL)
	if u == "" {
		return "", nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", nil
	}
	var doc struct {
		JWKS string `json:"jwks_uri"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return "", err
	}
	return strings.TrimSpace(doc.JWKS), nil
}
