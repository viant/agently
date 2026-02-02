package middleware

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	iauth "github.com/viant/agently/internal/auth"
	scyauth "github.com/viant/scy/auth"
	vcfg "github.com/viant/scy/auth/jwt/verifier"
	"golang.org/x/oauth2"
)

// Protect returns a middleware that, when enabled, requires authentication on
// /v1/api/* and /v1/workspace/*, except for /v1/api/auth/* and OPTIONS.
// It accepts either a valid cookie session (local/bff) or, when mode includes
// oidc, a Bearer token (TODO: integrate Datly auth validation).
func Protect(cfg *iauth.Config, sessions *iauth.Manager) func(http.Handler) http.Handler {
	enabled := cfg != nil && cfg.Enabled
	// mode no longer needed – use cfg helpers
	// Initialize OIDC verifier once from workspace config (JWKS)
	var verifier *vcfg.Service
	// 1) Prefer explicit oauth.client.jwksURL
	if cfg != nil && cfg.OAuth != nil && cfg.OAuth.Client != nil {
		jwks := strings.TrimSpace(cfg.OAuth.Client.JWKSURL)
		if jwks != "" {
			v := vcfg.New(&vcfg.Config{CertURL: jwks})
			if err := v.Init(context.Background()); err == nil {
				verifier = v
			}
		}
	}
	// 2) Resolve jwks from discoveryURL if present
	if verifier == nil && cfg != nil && cfg.OAuth != nil && cfg.OAuth.Client != nil && strings.TrimSpace(cfg.OAuth.Client.DiscoveryURL) != "" {
		if jwks, err := iauth.JWKSFromDiscovery(context.Background(), cfg.OAuth.Client.DiscoveryURL); err == nil && strings.TrimSpace(jwks) != "" {
			v := vcfg.New(&vcfg.Config{CertURL: jwks})
			if err := v.Init(context.Background()); err == nil {
				verifier = v
			}
		}
	}
	// 3) Derive jwks from OAuth client authorization URL (best-effort), when configURL available (BFF)
	if verifier == nil && cfg != nil && cfg.OAuth != nil && cfg.OAuth.Client != nil && strings.TrimSpace(cfg.OAuth.Client.ConfigURL) != "" {
		if jwks, err := iauth.JWKSFromBFFConfig(context.Background(), cfg.OAuth.Client.ConfigURL); err == nil && strings.TrimSpace(jwks) != "" {
			v := vcfg.New(&vcfg.Config{CertURL: jwks})
			if err := v.Init(context.Background()); err == nil {
				verifier = v
			}
		}
	}
	return func(next http.Handler) http.Handler {
		if !enabled {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// In local-only mode, do not enforce auth on any route, but ensure a user identity is present.
			if cfg.IsLocalAuth() {
				ctx := iauth.EnsureUser(r.Context(), cfg)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			path := r.URL.Path
			// Allow auth endpoints, CORS preflight, Forge UI metadata, and workspace metadata without auth
			if r.Method == http.MethodOptions || strings.HasPrefix(path, "/v1/api/auth/") || strings.HasPrefix(path, "/v1/api/agently/forge/") || path == "/v1/workspace/metadata" || (!isProtected(path)) {
				next.ServeHTTP(w, r)
				return
			}
			// Try Bearer when mode includes oidc. On success, attach bearer and minimal user info.
			if cfg.IsBearerAccepted() && hasBearer(r) {
				if validateBearerWithVerifier(r, verifier) {
					ctx := r.Context()
					authz := strings.TrimSpace(r.Header.Get("Authorization"))
					parts := strings.SplitN(authz, " ", 2)
					token := ""
					if len(parts) == 2 {
						token = strings.TrimSpace(parts[1])
					}
					if token != "" {
						ctx = iauth.WithBearer(ctx, token)
						// Store token bundle; for SPA flows Bearer can be either access or ID token.
						ctx = iauth.WithTokens(ctx, &scyauth.Token{Token: oauth2.Token{AccessToken: token, TokenType: "Bearer"}, IDToken: token})
						if info, _ := iauth.DecodeUserInfo(token); info != nil {
							ctx = iauth.WithUserInfo(ctx, info)
						}
					}
					// Ensure identity present if not derivable from token (rare)
					ctx = iauth.EnsureUser(ctx, cfg)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
			// Try cookie session for local/bff/mixed
			if cfg.IsCookieAccepted() {
				if sessions != nil {
					if sess := sessions.Get(r); sess != nil {
						ctx := iauth.WithUserInfo(r.Context(), &iauth.UserInfo{Subject: sess.UserID})
						// If session carries an access token (BFF), make it available to tools via context
						if sess.AccessToken != "" {
							ctx = iauth.WithBearer(ctx, sess.AccessToken)
						}
						if sess.IDToken != "" {
							ctx = iauth.WithIDToken(ctx, sess.IDToken)
						}
						if sess.AccessToken != "" || sess.IDToken != "" {
							ctx = iauth.WithTokens(ctx, &scyauth.Token{Token: oauth2.Token{AccessToken: sess.AccessToken, TokenType: "Bearer"}, IDToken: sess.IDToken})
						}
						ctx = iauth.EnsureUser(ctx, cfg)
						next.ServeHTTP(w, r.WithContext(ctx))
						return
					}
				}
				if cfg.IsLocalAuth() {
					unauthorized(w)
					return
				}
			}
			unauthorized(w)
		})
	}
}

func isProtected(path string) bool {
	return strings.HasPrefix(path, "/v1/api/") || strings.HasPrefix(path, "/v1/workspace/")
}

func hasBearer(r *http.Request) bool {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	return strings.HasPrefix(strings.ToLower(auth), "bearer ")
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ERROR", "message": "unauthorized"})
}

// validateBearerWithVerifier verifies token with a pre-initialized verifier.
func validateBearerWithVerifier(r *http.Request, v *vcfg.Service) bool {
	if v == nil {
		log.Printf("auth: bearer verification skipped (no verifier)")
		return false
	}
	authz := strings.TrimSpace(r.Header.Get("Authorization"))
	parts := strings.SplitN(authz, " ", 2)
	if len(parts) != 2 {
		log.Printf("auth: bearer verification failed (invalid Authorization header)")
		return false
	}
	token := strings.TrimSpace(parts[1])
	claims, err := v.VerifyClaims(r.Context(), token)
	if err != nil {
		log.Printf("auth: bearer verification failed: %v", err)
		return false
	}
	// Optional iss/aud checks from workspace config
	cfg := r.Context().Value(authConfigKey{}).(*iauth.Config)
	if cfg != nil && cfg.OAuth != nil && cfg.OAuth.Client != nil {
		iss := strings.TrimSpace(cfg.OAuth.Client.Issuer)
		if iss != "" && strings.TrimSpace(claims.Issuer) != iss {
			log.Printf("auth: bearer issuer mismatch (got=%q want=%q)", claims.Issuer, iss)
			return false
		}
		if len(cfg.OAuth.Client.Audiences) > 0 {
			ok := false
			for _, aud := range cfg.OAuth.Client.Audiences {
				if claims.VerifyAudience(strings.TrimSpace(aud), true) {
					ok = true
					break
				}
			}
			if !ok {
				log.Printf("auth: bearer audience mismatch (aud=%v)", cfg.OAuth.Client.Audiences)
				return false
			}
		}
	}
	return true
}

// authConfigKey is used to pass auth config via request context.
type authConfigKey struct{}

// WithAuthConfig injects auth config into request context for downstream checks.
func WithAuthConfig(next http.Handler, cfg *iauth.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), authConfigKey{}, cfg)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
