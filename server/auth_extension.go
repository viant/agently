package server

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	svcauth "github.com/viant/agently-core/service/auth"
	"github.com/viant/scy"
	scyauth "github.com/viant/scy/auth"
	"github.com/viant/scy/auth/authorizer"
	"github.com/viant/scy/auth/flow"
	"github.com/viant/scy/auth/jwt/signer"
	"github.com/viant/scy/kms"
	"github.com/viant/scy/kms/blowfish"
	"golang.org/x/oauth2"
)

type authExtension struct {
	cfg        *authConfig
	sessions   *svcauth.Manager
	jwtSignKey string
	tokenStore svcauth.TokenStore
}

func newAuthExtension(cfg *authConfig, sessions *svcauth.Manager, jwtSignKey string, tokenStore svcauth.TokenStore) *authExtension {
	if cfg == nil || sessions == nil {
		return nil
	}
	return &authExtension{
		cfg:        cfg,
		sessions:   sessions,
		jwtSignKey: strings.TrimSpace(jwtSignKey),
		tokenStore: tokenStore,
	}
}

func (a *authExtension) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/api/auth/me", a.handleMe())
	mux.HandleFunc("POST /v1/api/auth/local/login", a.handleLocalLogin())
	mux.HandleFunc("POST /v1/api/auth/logout", a.handleLogout())
	mux.HandleFunc("GET /v1/api/auth/providers", a.handleProviders())
	mux.HandleFunc("POST /v1/api/auth/session", a.handleCreateSession())
	mux.HandleFunc("GET /v1/api/auth/oauth/config", a.handleOAuthConfig())
	mux.HandleFunc("POST /v1/api/auth/oauth/initiate", a.handleOAuthInitiate())
	mux.HandleFunc("GET /v1/api/auth/oauth/callback", a.handleOAuthCallback())
	mux.HandleFunc("POST /v1/api/auth/oauth/callback", a.handleOAuthCallback())
	mux.HandleFunc("POST /v1/api/auth/oob", a.handleOAuthOOB())
	mux.HandleFunc("POST /v1/api/auth/idp/delegate", a.handleIDPDelegate())
	mux.HandleFunc("GET /v1/api/auth/idp/login", a.handleIDPLogin())
	mux.HandleFunc("POST /v1/api/auth/jwt/keypair", a.handleJWTKeyPair())
	mux.HandleFunc("POST /v1/api/auth/jwt/mint", a.handleJWTMint())
}

func (a *authExtension) handleMe() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess := a.currentSession(r)
		if sess == nil {
			if user := authUserFromContext(r.Context()); user != nil {
				httpJSON(w, http.StatusOK, map[string]any{
					"subject":     strings.TrimSpace(user.Subject),
					"username":    strings.TrimSpace(user.Subject),
					"email":       strings.TrimSpace(user.Email),
					"displayName": strings.TrimSpace(user.Subject),
					"provider":    "jwt",
				})
				return
			}
			httpError(w, http.StatusUnauthorized, fmt.Errorf("not authenticated"))
			return
		}
		if a.requiresOAuthTokens() && !a.ensureSessionOAuthTokens(r.Context(), sess) {
			if cookieName := strings.TrimSpace(a.cfg.CookieName); cookieName != "" {
				if c, err := r.Cookie(cookieName); err == nil && strings.TrimSpace(c.Value) != "" {
					a.sessions.Delete(r.Context(), strings.TrimSpace(c.Value))
				}
				http.SetCookie(w, &http.Cookie{
					Name:     cookieName,
					Value:    "",
					Path:     "/",
					HttpOnly: true,
					MaxAge:   -1,
				})
			}
			httpError(w, http.StatusUnauthorized, fmt.Errorf("oauth session is missing a valid token"))
			return
		}
		httpJSON(w, http.StatusOK, map[string]any{
			"subject":     strings.TrimSpace(sess.Subject),
			"username":    strings.TrimSpace(sess.Username),
			"email":       strings.TrimSpace(sess.Email),
			"displayName": strings.TrimSpace(sess.Username),
			"provider":    "session",
		})
	}
}

func (a *authExtension) handleLocalLogin() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.cfg == nil || a.cfg.Local == nil || !a.cfg.Local.Enabled {
			httpError(w, http.StatusForbidden, fmt.Errorf("local auth is not enabled"))
			return
		}
		var in struct {
			Username string `json:"username"`
			Name     string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			httpError(w, http.StatusBadRequest, err)
			return
		}
		username := strings.TrimSpace(in.Username)
		if username == "" {
			username = strings.TrimSpace(in.Name)
		}
		if username == "" {
			httpError(w, http.StatusBadRequest, fmt.Errorf("username is required"))
			return
		}
		sess := &svcauth.Session{
			ID:        uuid.New().String(),
			Username:  username,
			Subject:   username,
			CreatedAt: time.Now(),
		}
		a.sessions.Put(r.Context(), sess)
		writeSessionCookie(w, a.cfg, a.sessions, sess.ID)
		httpJSON(w, http.StatusOK, map[string]any{
			"sessionId": sess.ID,
			"username":  username,
			"provider":  "local",
		})
	}
}

func (a *authExtension) handleLogout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cookieName := strings.TrimSpace(a.cfg.CookieName); cookieName != "" {
			if c, err := r.Cookie(cookieName); err == nil && strings.TrimSpace(c.Value) != "" {
				a.sessions.Delete(r.Context(), c.Value)
			}
			http.SetCookie(w, &http.Cookie{
				Name:     cookieName,
				Value:    "",
				Path:     "/",
				HttpOnly: true,
				MaxAge:   -1,
			})
		}
		httpJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	}
}

func (a *authExtension) handleProviders() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		providers := make([]map[string]any, 0, 3)
		if a.cfg != nil && a.cfg.Local != nil && a.cfg.Local.Enabled {
			item := map[string]any{
				"name":  "local",
				"label": "Local User",
				"type":  "local",
			}
			if strings.TrimSpace(a.cfg.DefaultUsername) != "" {
				item["defaultUsername"] = strings.TrimSpace(a.cfg.DefaultUsername)
			}
			providers = append(providers, item)
		}
		if a.cfg != nil && a.cfg.OAuth != nil && a.cfg.OAuth.Client != nil {
			mode := strings.ToLower(strings.TrimSpace(a.cfg.OAuth.Mode))
			if mode == "bff" || mode == "mixed" {
				providers = append(providers, map[string]any{
					"name":  a.oauthProviderName(),
					"label": firstNonEmpty(a.cfg.OAuth.Label, "OAuth2"),
					"type":  "bff",
				})
			}
			if mode == "spa" || mode == "bearer" || mode == "oidc" || mode == "mixed" {
				providers = append(providers, map[string]any{
					"name":         a.oauthProviderName(),
					"label":        firstNonEmpty(a.cfg.OAuth.Label, "OIDC"),
					"type":         "oidc",
					"clientID":     strings.TrimSpace(a.cfg.OAuth.Client.ClientID),
					"discoveryURL": strings.TrimSpace(a.cfg.OAuth.Client.DiscoveryURL),
					"redirectURI":  strings.TrimSpace(a.cfg.OAuth.Client.RedirectURI),
					"scopes":       append([]string(nil), a.cfg.OAuth.Client.Scopes...),
				})
			}
		}
		if a.cfg != nil && a.cfg.JWT != nil && a.cfg.JWT.Enabled {
			providers = append(providers, map[string]any{
				"name":  "jwt",
				"label": "JWT",
				"type":  "jwt",
			})
		}
		httpJSON(w, http.StatusOK, providers)
	}
}

func (a *authExtension) handleOAuthConfig() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if a.cfg == nil || a.cfg.OAuth == nil || a.cfg.OAuth.Client == nil {
			httpJSON(w, http.StatusOK, map[string]any{})
			return
		}
		httpJSON(w, http.StatusOK, map[string]any{
			"mode":         strings.TrimSpace(a.cfg.OAuth.Mode),
			"configURL":    strings.TrimSpace(a.cfg.OAuth.Client.ConfigURL),
			"clientID":     strings.TrimSpace(a.cfg.OAuth.Client.ClientID),
			"discoveryURL": strings.TrimSpace(a.cfg.OAuth.Client.DiscoveryURL),
			"redirectURI":  strings.TrimSpace(a.cfg.OAuth.Client.RedirectURI),
			"scopes":       append([]string(nil), a.cfg.OAuth.Client.Scopes...),
		})
	}
}

func (a *authExtension) handleCreateSession() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			httpError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
			return
		}
		var in struct {
			Username     string `json:"username"`
			AccessToken  string `json:"accessToken,omitempty"`
			IDToken      string `json:"idToken,omitempty"`
			RefreshToken string `json:"refreshToken,omitempty"`
			ExpiresAt    string `json:"expiresAt,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			httpError(w, http.StatusBadRequest, err)
			return
		}
		bearerToken := bearerTokenFromRequest(r)
		if strings.TrimSpace(in.IDToken) == "" && strings.TrimSpace(in.AccessToken) == "" && bearerToken != "" {
			in.IDToken = bearerToken
			in.AccessToken = bearerToken
		}
		username := strings.TrimSpace(in.Username)
		subject := ""
		email := ""
		if username == "" {
			username, subject, email, _ = identityFromTokenStrings(strings.TrimSpace(in.IDToken), strings.TrimSpace(in.AccessToken))
		} else {
			_, subject, email, _ = identityFromTokenStrings(strings.TrimSpace(in.IDToken), strings.TrimSpace(in.AccessToken))
		}
		if username == "" {
			username = "user"
		}
		if subject == "" {
			subject = username
		}
		sess := &svcauth.Session{
			ID:        uuid.New().String(),
			Username:  username,
			Email:     email,
			Subject:   subject,
			CreatedAt: time.Now(),
		}
		if strings.TrimSpace(in.AccessToken) != "" || strings.TrimSpace(in.IDToken) != "" || strings.TrimSpace(in.RefreshToken) != "" {
			sess.Tokens = &scyauth.Token{
				Token: oauth2.Token{
					AccessToken:  strings.TrimSpace(in.AccessToken),
					RefreshToken: strings.TrimSpace(in.RefreshToken),
				},
				IDToken: strings.TrimSpace(in.IDToken),
			}
			if expiry := strings.TrimSpace(in.ExpiresAt); expiry != "" {
				if parsed, err := time.Parse(time.RFC3339, expiry); err == nil {
					sess.Tokens.Expiry = parsed
				}
			}
		}
		a.sessions.Put(r.Context(), sess)
		if a.tokenStore != nil && sess.Tokens != nil {
			_ = a.tokenStore.Put(r.Context(), &svcauth.OAuthToken{
				Username:     firstNonEmpty(subject, username),
				Provider:     a.oauthProviderName(),
				AccessToken:  strings.TrimSpace(in.AccessToken),
				IDToken:      strings.TrimSpace(in.IDToken),
				RefreshToken: strings.TrimSpace(in.RefreshToken),
				ExpiresAt:    sess.Tokens.Expiry,
			})
		}
		writeSessionCookie(w, a.cfg, a.sessions, sess.ID)
		httpJSON(w, http.StatusOK, map[string]any{
			"sessionId": sess.ID,
			"username":  username,
		})
	}
}

func (a *authExtension) handleIDPDelegate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp, err := a.buildOAuthInitiateResponse(r)
		if err != nil {
			httpError(w, http.StatusBadRequest, err)
			return
		}
		httpJSON(w, http.StatusOK, map[string]any{
			"mode":      "delegated",
			"idpLogin":  resp.AuthURL,
			"provider":  a.oauthProviderName(),
			"authURL":   resp.AuthURL,
			"state":     resp.State,
			"expiresIn": 300,
		})
	}
}

func (a *authExtension) handleIDPLogin() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp, err := a.buildOAuthInitiateResponse(r)
		if err != nil {
			httpError(w, http.StatusBadRequest, err)
			return
		}
		http.Redirect(w, r, resp.AuthURL, http.StatusTemporaryRedirect)
	}
}

func (a *authExtension) handleOAuthInitiate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp, err := a.buildOAuthInitiateResponse(r)
		if err != nil {
			httpError(w, http.StatusBadRequest, err)
			return
		}
		httpJSON(w, http.StatusOK, map[string]any{
			"authURL":   resp.AuthURL,
			"state":     resp.State,
			"provider":  a.oauthProviderName(),
			"delegated": true,
		})
	}
}

func (a *authExtension) handleOAuthOOB() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.cfg == nil || a.cfg.OAuth == nil || a.cfg.OAuth.Client == nil {
			httpError(w, http.StatusBadRequest, fmt.Errorf("oauth client not configured"))
			return
		}
		var in struct {
			SecretsURL string   `json:"secretsURL"`
			Scopes     []string `json:"scopes,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			httpError(w, http.StatusBadRequest, err)
			return
		}
		secretsURL := strings.TrimSpace(in.SecretsURL)
		if secretsURL == "" {
			httpError(w, http.StatusBadRequest, fmt.Errorf("secretsURL is required"))
			return
		}
		configURL := strings.TrimSpace(a.cfg.OAuth.Client.ConfigURL)
		if configURL == "" {
			httpError(w, http.StatusBadRequest, fmt.Errorf("oauth client configURL is required"))
			return
		}
		scopes := in.Scopes
		if len(scopes) == 0 {
			scopes = append([]string(nil), a.cfg.OAuth.Client.Scopes...)
		}
		if len(scopes) == 0 {
			scopes = []string{"openid"}
		}
		cmd := &authorizer.Command{
			AuthFlow:   "OOB",
			UsePKCE:    true,
			SecretsURL: secretsURL,
			Scopes:     scopes,
			OAuthConfig: authorizer.OAuthConfig{
				ConfigURL: configURL,
			},
		}
		token, err := authorizer.New().Authorize(r.Context(), cmd)
		if err != nil {
			httpError(w, http.StatusUnauthorized, err)
			return
		}
		if token == nil {
			httpError(w, http.StatusUnauthorized, fmt.Errorf("oauth oob returned empty token"))
			return
		}
		username, subject, email, idToken := identityFromOAuthToken(token)
		if username == "" {
			username = "user"
		}
		provider := a.oauthProviderName()
		sess := &svcauth.Session{
			ID:        uuid.New().String(),
			Username:  username,
			Email:     email,
			Subject:   subject,
			CreatedAt: time.Now(),
			Tokens: &scyauth.Token{
				Token: oauth2.Token{
					AccessToken:  token.AccessToken,
					RefreshToken: token.RefreshToken,
					Expiry:       token.Expiry,
				},
				IDToken: idToken,
			},
		}
		a.sessions.Put(r.Context(), sess)
		writeSessionCookie(w, a.cfg, a.sessions, sess.ID)
		if a.tokenStore != nil {
			_ = a.tokenStore.Put(r.Context(), &svcauth.OAuthToken{
				Username:     firstNonEmpty(subject, username),
				Provider:     provider,
				AccessToken:  token.AccessToken,
				IDToken:      idToken,
				RefreshToken: token.RefreshToken,
				ExpiresAt:    token.Expiry,
			})
		}
		httpJSON(w, http.StatusOK, map[string]any{
			"status":   "ok",
			"username": username,
			"provider": provider,
		})
	}
}

type oauthInitiateResponse struct {
	AuthURL string
	State   string
}

type oauthStatePayload struct {
	CodeVerifier string `json:"codeVerifier"`
	ReturnURL    string `json:"returnURL,omitempty"`
}

func (a *authExtension) buildOAuthInitiateResponse(r *http.Request) (*oauthInitiateResponse, error) {
	if a.cfg == nil || a.cfg.OAuth == nil || a.cfg.OAuth.Client == nil {
		return nil, fmt.Errorf("oauth client not configured")
	}
	configURL := strings.TrimSpace(a.cfg.OAuth.Client.ConfigURL)
	if configURL == "" {
		return nil, fmt.Errorf("oauth client configURL is required for delegated login")
	}
	oauthCfg, err := loadOAuthClientConfig(r.Context(), configURL)
	if err != nil {
		return nil, fmt.Errorf("unable to load oauth config: %w", err)
	}
	redirectURI := callbackURL(r, a.cfg.RedirectPath)
	codeVerifier := flow.GenerateCodeVerifier()
	returnURL := strings.TrimSpace(r.URL.Query().Get("returnURL"))
	state, err := encryptOAuthState(r.Context(), configURL, oauthStatePayload{
		CodeVerifier: codeVerifier,
		ReturnURL:    returnURL,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to create oauth state: %w", err)
	}
	scopes := a.cfg.OAuth.Client.Scopes
	if len(scopes) == 0 {
		scopes = []string{"openid", "profile", "email"}
	}
	authURL, err := flow.BuildAuthCodeURL(oauthCfg,
		flow.WithPKCE(true),
		flow.WithState(state),
		flow.WithRedirectURI(redirectURI),
		flow.WithScopes(scopes...),
		flow.WithCodeVerifier(codeVerifier),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to build oauth authorize url: %w", err)
	}
	return &oauthInitiateResponse{AuthURL: authURL, State: state}, nil
}

func (a *authExtension) handleOAuthCallback() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.cfg == nil || a.cfg.OAuth == nil || a.cfg.OAuth.Client == nil {
			httpError(w, http.StatusBadRequest, fmt.Errorf("oauth client not configured"))
			return
		}
		configURL := strings.TrimSpace(a.cfg.OAuth.Client.ConfigURL)
		if configURL == "" {
			httpError(w, http.StatusBadRequest, fmt.Errorf("oauth client configURL is required"))
			return
		}
		code := strings.TrimSpace(r.URL.Query().Get("code"))
		state := strings.TrimSpace(r.URL.Query().Get("state"))
		if code == "" || state == "" {
			var body struct {
				Code  string `json:"code"`
				State string `json:"state"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
				if code == "" {
					code = strings.TrimSpace(body.Code)
				}
				if state == "" {
					state = strings.TrimSpace(body.State)
				}
			}
		}
		if code == "" || state == "" {
			httpError(w, http.StatusBadRequest, fmt.Errorf("missing oauth code/state"))
			return
		}
		oauthCfg, err := loadOAuthClientConfig(r.Context(), configURL)
		if err != nil {
			httpError(w, http.StatusBadRequest, fmt.Errorf("unable to load oauth config: %w", err))
			return
		}
		statePayload, err := decryptOAuthState(r.Context(), configURL, state)
		if err != nil {
			httpError(w, http.StatusBadRequest, fmt.Errorf("invalid oauth state: %w", err))
			return
		}
		codeVerifier := strings.TrimSpace(statePayload.CodeVerifier)
		if codeVerifier == "" {
			httpError(w, http.StatusBadRequest, fmt.Errorf("invalid oauth state: missing code verifier"))
			return
		}
		redirectURI := callbackURL(r, a.cfg.RedirectPath)
		token, err := flow.Exchange(r.Context(), oauthCfg, code,
			flow.WithRedirectURI(redirectURI),
			flow.WithPKCE(true),
			flow.WithCodeVerifier(codeVerifier),
		)
		if err != nil {
			httpError(w, http.StatusUnauthorized, fmt.Errorf("oauth exchange failed: %w", err))
			return
		}
		username, subject, email, idToken := identityFromOAuthToken(token)
		if username == "" {
			username = "user"
		}
		provider := a.oauthProviderName()
		sess := &svcauth.Session{
			ID:        uuid.New().String(),
			Username:  username,
			Email:     email,
			Subject:   subject,
			CreatedAt: time.Now(),
			Tokens: &scyauth.Token{
				Token: oauth2.Token{
					AccessToken:  token.AccessToken,
					RefreshToken: token.RefreshToken,
					Expiry:       token.Expiry,
				},
				IDToken: idToken,
			},
		}
		a.sessions.Put(r.Context(), sess)
		writeSessionCookie(w, a.cfg, a.sessions, sess.ID)
		if a.tokenStore != nil {
			_ = a.tokenStore.Put(r.Context(), &svcauth.OAuthToken{
				Username:     firstNonEmpty(subject, username),
				Provider:     provider,
				AccessToken:  token.AccessToken,
				IDToken:      idToken,
				RefreshToken: token.RefreshToken,
				ExpiresAt:    token.Expiry,
			})
		}
		if wantsJSON(r) {
			httpJSON(w, http.StatusOK, map[string]any{"status": "ok", "username": username, "provider": provider})
			return
		}
		// Redirect back to the app. If a returnURL was embedded in the state, use it;
		// otherwise fall back to root. This supports both same-tab BFF flows and
		// full-page redirect flows (RedirectSameTab=true or BFF default).
		returnTo := strings.TrimSpace(statePayload.ReturnURL)
		if returnTo == "" {
			returnTo = "/"
		}
		http.Redirect(w, r, returnTo, http.StatusTemporaryRedirect)
	}
}

func (a *authExtension) handleJWTKeyPair() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Bits           int    `json:"bits"`
			PrivateKeyPath string `json:"privateKeyPath,omitempty"`
			PublicKeyPath  string `json:"publicKeyPath,omitempty"`
			Overwrite      bool   `json:"overwrite,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			httpError(w, http.StatusBadRequest, err)
			return
		}
		if in.Bits <= 0 {
			in.Bits = 2048
		}
		key, err := rsa.GenerateKey(rand.Reader, in.Bits)
		if err != nil {
			httpError(w, http.StatusInternalServerError, fmt.Errorf("unable to generate rsa key: %w", err))
			return
		}
		privatePEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
		publicDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
		if err != nil {
			httpError(w, http.StatusInternalServerError, fmt.Errorf("unable to encode public key: %w", err))
			return
		}
		publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})
		if err := writePEMFiles(in.PrivateKeyPath, in.PublicKeyPath, privatePEM, publicPEM, in.Overwrite); err != nil {
			httpError(w, http.StatusBadRequest, err)
			return
		}
		httpJSON(w, http.StatusOK, map[string]any{
			"privateKey":     string(privatePEM),
			"publicKey":      string(publicPEM),
			"privateKeyPath": strings.TrimSpace(in.PrivateKeyPath),
			"publicKeyPath":  strings.TrimSpace(in.PublicKeyPath),
			"bits":           in.Bits,
		})
	}
}

func (a *authExtension) handleJWTMint() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			PrivateKeyPath string                 `json:"privateKeyPath,omitempty"`
			Subject        string                 `json:"subject,omitempty"`
			Email          string                 `json:"email,omitempty"`
			Username       string                 `json:"username,omitempty"`
			Name           string                 `json:"name,omitempty"`
			TTLSeconds     int                    `json:"ttlSeconds,omitempty"`
			Claims         map[string]interface{} `json:"claims,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			httpError(w, http.StatusBadRequest, err)
			return
		}
		ttl := time.Hour
		if in.TTLSeconds > 0 {
			ttl = time.Duration(in.TTLSeconds) * time.Second
		}
		claims := map[string]interface{}{}
		for k, v := range in.Claims {
			claims[k] = v
		}
		if strings.TrimSpace(in.Subject) != "" {
			claims["sub"] = strings.TrimSpace(in.Subject)
		}
		if strings.TrimSpace(in.Email) != "" {
			claims["email"] = strings.TrimSpace(in.Email)
		}
		if strings.TrimSpace(in.Username) != "" {
			claims["username"] = strings.TrimSpace(in.Username)
		}
		if strings.TrimSpace(in.Name) != "" {
			claims["name"] = strings.TrimSpace(in.Name)
		}
		var (
			token string
			err   error
		)
		privatePath := strings.TrimSpace(in.PrivateKeyPath)
		if privatePath != "" {
			token, err = signWithPrivateKey(r.Context(), privatePath, ttl, claims)
		} else if a.jwtSignKey != "" {
			token, err = signWithPrivateKey(r.Context(), a.jwtSignKey, ttl, claims)
		} else {
			err = fmt.Errorf("jwt signer not configured; set auth.jwt.rsaPrivateKey or provide privateKeyPath")
		}
		if err != nil {
			httpError(w, http.StatusBadRequest, err)
			return
		}
		httpJSON(w, http.StatusOK, map[string]any{
			"token":      token,
			"tokenType":  "Bearer",
			"expiresAt":  time.Now().Add(ttl).UTC().Format(time.RFC3339),
			"ttlSeconds": int(ttl.Seconds()),
		})
	}
}

func signWithPrivateKey(ctx context.Context, privateKeyPath string, ttl time.Duration, claims map[string]interface{}) (string, error) {
	cfg := &signer.Config{RSA: scy.NewResource("", privateKeyPath, "")}
	s := signer.New(cfg)
	if err := s.Init(ctx); err != nil {
		return "", fmt.Errorf("unable to init jwt signer: %w", err)
	}
	token, err := s.Create(ttl, claims)
	if err != nil {
		return "", fmt.Errorf("unable to sign jwt: %w", err)
	}
	return token, nil
}

func writePEMFiles(privatePath, publicPath string, privatePEM, publicPEM []byte, overwrite bool) error {
	privatePath = strings.TrimSpace(privatePath)
	publicPath = strings.TrimSpace(publicPath)
	if privatePath == "" && publicPath == "" {
		return nil
	}
	writeFile := func(path string, mode os.FileMode, data []byte) error {
		if path == "" {
			return nil
		}
		if !overwrite {
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("file already exists: %s", path)
			}
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		return os.WriteFile(path, data, mode)
	}
	if err := writeFile(privatePath, 0o600, privatePEM); err != nil {
		return err
	}
	if err := writeFile(publicPath, 0o644, publicPEM); err != nil {
		return err
	}
	return nil
}

func wantsJSON(r *http.Request) bool {
	if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("format")), "json") {
		return true
	}
	return strings.Contains(strings.ToLower(r.Header.Get("Accept")), "application/json")
}

func callbackURL(r *http.Request, configuredPath string) string {
	scheme := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	path := strings.TrimSpace(configuredPath)
	if path == "" {
		path = "/v1/api/auth/oauth/callback"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return scheme + "://" + host + path
}

func identityFromOAuthToken(tok *oauth2.Token) (username, subject, email, idToken string) {
	if tok == nil {
		return "", "", "", ""
	}
	if raw := tok.Extra("id_token"); raw != nil {
		if s, ok := raw.(string); ok {
			idToken = strings.TrimSpace(s)
		}
	}
	return identityFromTokenStrings(idToken, tok.AccessToken)
}

func identityFromTokenStrings(idToken, accessToken string) (username, subject, email, rawID string) {
	rawID = strings.TrimSpace(idToken)
	claims := parseJWTClaims(rawID)
	if len(claims) == 0 {
		claims = parseJWTClaims(strings.TrimSpace(accessToken))
	}
	subject = claimString(claims, "sub")
	email = claimString(claims, "email")
	username = strings.TrimSpace(claimString(claims, "preferred_username"))
	if username == "" {
		username = strings.TrimSpace(claimString(claims, "name"))
	}
	if username == "" && email != "" {
		if idx := strings.Index(email, "@"); idx > 0 {
			username = email[:idx]
		} else {
			username = email
		}
	}
	if username == "" {
		username = subject
	}
	return username, subject, email, rawID
}

func parseJWTClaims(token string) map[string]interface{} {
	token = strings.TrimSpace(token)
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return map[string]interface{}{}
	}
	seg := parts[1]
	switch len(seg) % 4 {
	case 2:
		seg += "=="
	case 3:
		seg += "="
	}
	data, err := base64.URLEncoding.DecodeString(seg)
	if err != nil {
		return map[string]interface{}{}
	}
	out := map[string]interface{}{}
	if err := json.Unmarshal(data, &out); err != nil {
		return map[string]interface{}{}
	}
	return out
}

func bearerTokenFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(header) < 8 || !strings.EqualFold(header[:7], "Bearer ") {
		return ""
	}
	return strings.TrimSpace(header[7:])
}

func claimString(claims map[string]interface{}, key string) string {
	raw, ok := claims[key]
	if !ok {
		return ""
	}
	val, ok := raw.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(val)
}

func writeSessionCookie(w http.ResponseWriter, cfg *authConfig, sessions *svcauth.Manager, sessionID string) {
	if cfg == nil || strings.TrimSpace(cfg.CookieName) == "" || sessions == nil {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cfg.CookieName,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionsTTLSeconds(cfg)),
	})
}

func sessionsTTLSeconds(cfg *authConfig) int64 {
	hours := cfg.SessionTTLHours
	if hours <= 0 {
		hours = 24 * 7
	}
	return int64(time.Duration(hours) * time.Hour / time.Second)
}

func (a *authExtension) oauthProviderName() string {
	if a.cfg == nil || a.cfg.OAuth == nil {
		return "oauth"
	}
	if name := strings.TrimSpace(a.cfg.OAuth.Name); name != "" {
		return name
	}
	return "oauth"
}

func (a *authExtension) currentSession(r *http.Request) *svcauth.Session {
	if a == nil || a.sessions == nil || a.cfg == nil {
		return nil
	}
	cookieName := strings.TrimSpace(a.cfg.CookieName)
	if cookieName == "" {
		return nil
	}
	c, err := r.Cookie(cookieName)
	if err != nil {
		return nil
	}
	id := strings.TrimSpace(c.Value)
	if id == "" {
		return nil
	}
	return a.sessions.Get(r.Context(), id)
}

func (a *authExtension) requiresOAuthTokens() bool {
	if a == nil || a.cfg == nil || a.cfg.OAuth == nil {
		return false
	}
	mode := strings.ToLower(strings.TrimSpace(a.cfg.OAuth.Mode))
	return mode == "bff" || mode == "mixed"
}

func (a *authExtension) ensureSessionOAuthTokens(ctx context.Context, sess *svcauth.Session) bool {
	if sess == nil {
		return false
	}
	if sess.Tokens != nil {
		if strings.TrimSpace(sess.Tokens.AccessToken) != "" || strings.TrimSpace(sess.Tokens.IDToken) != "" {
			return true
		}
	}
	if a == nil || a.tokenStore == nil {
		return false
	}
	username := strings.TrimSpace(firstNonEmpty(sess.Subject, sess.Username))
	if username == "" {
		return false
	}
	provider := a.oauthProviderName()
	dbTok, err := a.tokenStore.Get(ctx, username, provider)
	if err != nil || dbTok == nil {
		return false
	}
	if dbTok.ExpiresAt.IsZero() || !dbTok.ExpiresAt.After(time.Now()) {
		return false
	}
	sess.Tokens = &scyauth.Token{
		Token: oauth2.Token{
			AccessToken:  dbTok.AccessToken,
			RefreshToken: dbTok.RefreshToken,
			Expiry:       dbTok.ExpiresAt,
		},
		IDToken: dbTok.IDToken,
	}
	a.sessions.Put(ctx, sess)
	return true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

var stateCipher = blowfish.Cipher{}

func encryptState(ctx context.Context, salt, value string) (string, error) {
	key := &kms.Key{Kind: "raw", Raw: string(blowfish.EnsureKey([]byte(strings.TrimSpace(salt))))}
	encrypted, err := stateCipher.Encrypt(ctx, key, []byte(value))
	if err != nil {
		return "", err
	}
	return strings.TrimRight(base64.URLEncoding.EncodeToString(encrypted), "="), nil
}

func encryptOAuthState(ctx context.Context, salt string, payload oauthStatePayload) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return encryptState(ctx, salt, string(data))
}

func decryptState(ctx context.Context, salt, state string) (string, error) {
	raw := strings.TrimSpace(state)
	switch len(raw) % 4 {
	case 2:
		raw += "=="
	case 3:
		raw += "="
	}
	data, err := base64.URLEncoding.DecodeString(raw)
	if err != nil {
		return "", err
	}
	key := &kms.Key{Kind: "raw", Raw: string(blowfish.EnsureKey([]byte(strings.TrimSpace(salt))))}
	decrypted, err := stateCipher.Decrypt(ctx, key, data)
	if err != nil {
		return "", err
	}
	return string(decrypted), nil
}

func decryptOAuthState(ctx context.Context, salt, state string) (oauthStatePayload, error) {
	raw, err := decryptState(ctx, salt, state)
	if err != nil {
		return oauthStatePayload{}, err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return oauthStatePayload{}, fmt.Errorf("empty state payload")
	}
	if strings.HasPrefix(raw, "{") {
		var payload oauthStatePayload
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return oauthStatePayload{}, err
		}
		return payload, nil
	}
	return oauthStatePayload{CodeVerifier: raw}, nil
}

func loadOAuthClientConfig(ctx context.Context, configURL string) (*oauth2.Config, error) {
	oa := authorizer.New()
	oc := &authorizer.OAuthConfig{ConfigURL: configURL}
	if err := oa.EnsureConfig(ctx, oc); err == nil && oc.Config != nil {
		return oc.Config, nil
	}
	path := strings.TrimSpace(configURL)
	if strings.HasPrefix(path, "file://") {
		if u, err := url.Parse(path); err == nil {
			path = u.Path
		}
	}
	if strings.Contains(path, "://") && !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("unsupported oauth config url: %s", configURL)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw struct {
		AuthURL      string   `json:"authURL"`
		TokenURL     string   `json:"tokenURL"`
		ClientID     string   `json:"clientID"`
		ClientSecret string   `json:"clientSecret"`
		RedirectURL  string   `json:"redirectURL"`
		Scopes       []string `json:"scopes"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw.AuthURL) == "" || strings.TrimSpace(raw.TokenURL) == "" || strings.TrimSpace(raw.ClientID) == "" {
		return nil, fmt.Errorf("oauth config requires authURL, tokenURL, and clientID")
	}
	cfg := &oauth2.Config{
		ClientID:     strings.TrimSpace(raw.ClientID),
		ClientSecret: strings.TrimSpace(raw.ClientSecret),
		RedirectURL:  strings.TrimSpace(raw.RedirectURL),
		Scopes:       append([]string(nil), raw.Scopes...),
		Endpoint: oauth2.Endpoint{
			AuthURL:  strings.TrimSpace(raw.AuthURL),
			TokenURL: strings.TrimSpace(raw.TokenURL),
		},
	}
	return cfg, nil
}

func httpJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func httpError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  "error",
		"message": err.Error(),
	})
}
