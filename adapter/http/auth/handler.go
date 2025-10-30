package auth

import (
	"encoding/json"
	"net/http"
	"strings"

	"context"
	"encoding/base64"
	"github.com/golang-jwt/jwt/v5"
	iauth "github.com/viant/agently/internal/auth"
	usersvc "github.com/viant/agently/internal/service/user"
	userread "github.com/viant/agently/pkg/agently/user"
	"github.com/viant/datly"
	"github.com/viant/scy/auth/authorizer"
	"github.com/viant/scy/auth/flow"
	vcfg "github.com/viant/scy/auth/jwt/verifier"
	"github.com/viant/scy/kms"
	"github.com/viant/scy/kms/blowfish"
	"net/url"
	"path"
)

// Service bundles deps for auth endpoints.
type Service struct {
	cfg      *iauth.Config
	sess     *iauth.Manager
	users    *usersvc.Service
	defModel string
	defAgent string
	defEmbed string
	dao      *datly.Service
}

// NewWithDatly removed; keep stub to avoid breaking imports.
func NewWithDatly(_ *datly.Service) (http.Handler, error) {
	return nil, errf("auth: config must be supplied by caller")
}

// New kept for backwards-compatibility; prefers single init but constructs when needed.
func New() (http.Handler, error) { return nil, errf("auth: config must be supplied by caller") }

// NewWithDatlyAndConfig allows sharing a single session manager and config across middleware and handlers.
func NewWithDatlyAndConfig(dao *datly.Service, sess *iauth.Manager, cfg *iauth.Config) (http.Handler, error) {
	return NewWithDatlyAndConfigExt(dao, sess, cfg, "", "", "")
}

// NewWithDatlyAndConfigExt allows passing workspace defaults (model/agent) to enrich /auth/me.
func NewWithDatlyAndConfigExt(dao *datly.Service, sess *iauth.Manager, cfg *iauth.Config, defaultModel, defaultAgent, defaultEmbedder string) (http.Handler, error) {
	if cfg == nil {
		return nil, errf("auth: nil config")
	}
	users, err := usersvc.New(context.Background(), dao)
	if err != nil {
		return nil, err
	}
	s := &Service{cfg: cfg, sess: sess, users: users, defModel: strings.TrimSpace(defaultModel), defAgent: strings.TrimSpace(defaultAgent), defEmbed: strings.TrimSpace(defaultEmbedder), dao: dao}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/api/auth/local/login", s.handleLocalLogin)
	mux.HandleFunc("/v1/api/auth/me", s.handleMe)
	mux.HandleFunc("/v1/api/auth/logout", s.handleLogout)
	mux.HandleFunc("/v1/api/auth/providers", s.handleProviders)
	mux.HandleFunc("/v1/api/auth/oauth/initiate", s.handleOAuthInitiate)
	mux.HandleFunc("/v1/api/auth/oauth/callback", s.handleOAuthCallback)
	return mux, nil
}

func (s *Service) handleLocalLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.cfg == nil || !s.cfg.Enabled || s.cfg.Local == nil || !s.cfg.Local.Enabled {
		encode(w, http.StatusForbidden, nil, errf("local auth disabled"))
		return
	}
	var payload struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		encode(w, http.StatusBadRequest, nil, err)
		return
	}
	name := strings.TrimSpace(payload.Name)
	if name == "" {
		encode(w, http.StatusBadRequest, nil, errf("name is required"))
		return
	}
	// Upsert user
	id, err := s.users.UpsertLocal(r.Context(), name, name, "")
	if err != nil {
		encode(w, 0, nil, err)
		return
	}
	// Compute and store hash_ip
	if s.cfg != nil && strings.TrimSpace(s.cfg.IpHashKey) != "" {
		ip := iauth.ClientIP(r, s.cfg.TrustedProxies)
		if h := iauth.HashIP(ip, s.cfg.IpHashKey); h != "" {
			_ = s.users.UpdateHashIPByID(r.Context(), id, h)
		}
	}
	// Ensure defaults populated at creation time if empty
	if item2, _ := s.users.FindByUsername(r.Context(), name); item2 != nil {
		var da, dm, de *string
		if item2.DefaultAgentRef == nil || strings.TrimSpace(*item2.DefaultAgentRef) == "" {
			if s.defAgent != "" {
				v := s.defAgent
				da = &v
			}
		}
		if item2.DefaultModelRef == nil || strings.TrimSpace(*item2.DefaultModelRef) == "" {
			if s.defModel != "" {
				v := s.defModel
				dm = &v
			}
		}
		if item2.DefaultEmbedderRef == nil || strings.TrimSpace(*item2.DefaultEmbedderRef) == "" {
			if s.defEmbed != "" {
				v := s.defEmbed
				de = &v
			}
		}
		if da != nil || dm != nil || de != nil {
			_ = s.users.UpdatePreferencesByUsername(r.Context(), name, nil, nil, da, dm, de)
		}
	}
	// Create session
	s.sess.Create(w, name) // session userID uses username for local; later can be actual id
	encode(w, http.StatusOK, map[string]any{"name": name, "provider": "local"}, nil)
}

func (s *Service) handleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	session := s.sess.Get(r)
	var item *userread.UserView
	var err error
	if session == nil {
		// Support SPA (Bearer) without cookie: derive identity from context
		ui := iauth.User(r.Context())
		if ui == nil || (strings.TrimSpace(ui.Subject) == "" && strings.TrimSpace(ui.Email) == "") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		// Derive username: prefer email local-part, else subject
		uname := strings.TrimSpace(ui.Subject)
		if e := strings.TrimSpace(ui.Email); e != "" {
			if i := strings.Index(e, "@"); i > 0 {
				uname = e[:i]
			} else {
				uname = e
			}
		}
		if strings.TrimSpace(uname) == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		item, err = s.users.FindByUsername(r.Context(), uname)
		if err != nil {
			encode(w, 0, nil, err)
			return
		}
		if item == nil {
			// Upsert minimal user record for SPA identities
			_, err = s.users.UpsertWithProvider(r.Context(), uname, uname, ui.Email, "oauth", ui.Subject)
			if err != nil {
				encode(w, 0, nil, err)
				return
			}
			item, err = s.users.FindByUsername(r.Context(), uname)
			if err != nil || item == nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
	} else {
		item, err = s.users.FindByUsername(r.Context(), session.UserID)
	}
	if err != nil {
		encode(w, 0, nil, err)
		return
	}
	if item == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	// Enrich default refs from executor config when user fields are empty
	defAgent := value(item.DefaultAgentRef)
	if strings.TrimSpace(defAgent) == "" && strings.TrimSpace(s.defAgent) != "" {
		defAgent = s.defAgent
	}
	defModel := value(item.DefaultModelRef)
	if strings.TrimSpace(defModel) == "" && strings.TrimSpace(s.defModel) != "" {
		defModel = s.defModel
	}
	defEmbed := value(item.DefaultEmbedderRef)
	if strings.TrimSpace(defEmbed) == "" && strings.TrimSpace(s.defEmbed) != "" {
		defEmbed = s.defEmbed
	}

	data := map[string]any{
		"username":           item.Username,
		"displayName":        coalesce(item.DisplayName, item.Username),
		"email":              value(item.Email),
		"provider":           item.Provider,
		"timezone":           item.Timezone,
		"defaultAgentRef":    defAgent,
		"defaultModelRef":    defModel,
		"defaultEmbedderRef": defEmbed,
	}
	encode(w, http.StatusOK, data, nil)
}

func (s *Service) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.sess.Destroy(w, r)
	encode(w, http.StatusOK, "ok", nil)
}

// OAuth (BFF) – initiate server-side Code+PKCE flow
func (s *Service) handleOAuthInitiate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.cfg == nil || s.cfg.OAuth == nil || s.cfg.OAuth.Client == nil || strings.TrimSpace(s.cfg.OAuth.Client.ConfigURL) == "" {
		encode(w, http.StatusBadRequest, nil, errf("oauth client not configured"))
		return
	}
	// Load OAuth client via scy
	oa := authorizer.New()
	oc := &authorizer.OAuthConfig{ConfigURL: s.cfg.OAuth.Client.ConfigURL}
	if err := oa.EnsureConfig(r.Context(), oc); err != nil {
		encode(w, 0, nil, err)
		return
	}
	// Build redirectURI (callback)
	cb := s.callbackURL(r)
	codeVerifier := flow.GenerateCodeVerifier()
	state := s.encryptState(r.Context(), codeVerifier)
	scopes := s.cfg.OAuth.Client.Scopes
	if len(scopes) == 0 {
		scopes = []string{"openid", "profile", "email"}
	}
	url, err := flow.BuildAuthCodeURL(oc.Config,
		flow.WithPKCE(true), flow.WithState(state), flow.WithRedirectURI(cb), flow.WithScopes(scopes...), flow.WithCodeVerifier(codeVerifier))
	if err != nil {
		encode(w, 0, nil, err)
		return
	}
	encode(w, http.StatusOK, map[string]any{"authURL": url}, nil)
}

// OAuth (BFF) – callback: exchange code, set cookie, upsert user, hash_ip
func (s *Service) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	code := strings.TrimSpace(q.Get("code"))
	state := strings.TrimSpace(q.Get("state"))
	if code == "" {
		var body struct{ Code, State string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		if code == "" {
			code = strings.TrimSpace(body.Code)
		}
		if state == "" {
			state = strings.TrimSpace(body.State)
		}
	}
	if code == "" || state == "" {
		encode(w, http.StatusBadRequest, nil, errf("missing code/state"))
		return
	}

	oa := authorizer.New()
	oc := &authorizer.OAuthConfig{ConfigURL: s.cfg.OAuth.Client.ConfigURL}
	if err := oa.EnsureConfig(r.Context(), oc); err != nil {
		encode(w, 0, nil, err)
		return
	}
	cb := s.callbackURL(r)
	// Decrypt state to get code_verifier
	codeVerifier, err := s.decryptState(r.Context(), state)
	if err != nil {
		encode(w, 0, nil, err)
		return
	}
	token, err := flow.Exchange(r.Context(), oc.Config, code, flow.WithRedirectURI(cb), flow.WithPKCE(true), flow.WithCodeVerifier(codeVerifier))
	if err != nil {
		encode(w, 0, nil, err)
		return
	}
	// Extract ID token and verify claims (signature + optional iss/aud checks)
	var sub, email, name, idTokenStr string
	if raw := token.Extra("id_token"); raw != nil {
		if idToken, ok := raw.(string); ok && idToken != "" {
			idTokenStr = idToken
			sVal, eVal, nVal, vErr := s.verifyIDToken(r.Context(), idToken)
			if vErr != nil {
				encode(w, 0, nil, vErr)
				return
			}
			sub, email, name = sVal, eVal, nVal
		}
	}
	username := strings.TrimSpace(name)
	if username == "" && email != "" {
		if i := strings.Index(email, "@"); i > 0 {
			username = email[:i]
		}
	}
	if username == "" && sub != "" {
		username = sub
	}
	if username == "" {
		username = "user"
	}

	// Upsert user with provider 'oauth'
	id, err := s.users.UpsertWithProvider(r.Context(), username, name, email, "oauth", sub)
	if err != nil {
		encode(w, 0, nil, err)
		return
	}
	// Compute hash_ip and set session
	if s.cfg != nil && strings.TrimSpace(s.cfg.IpHashKey) != "" {
		ip := iauth.ClientIP(r, s.cfg.TrustedProxies)
		if h := iauth.HashIP(ip, s.cfg.IpHashKey); h != "" {
			_ = s.users.UpdateHashIPByID(r.Context(), id, h)
		}
	}
	// Ensure defaults populated at creation time if empty
	if item2, _ := s.users.FindByUsername(r.Context(), username); item2 != nil {
		var da, dm, de *string
		if item2.DefaultAgentRef == nil || strings.TrimSpace(*item2.DefaultAgentRef) == "" {
			if s.defAgent != "" {
				v := s.defAgent
				da = &v
			}
		}
		if item2.DefaultModelRef == nil || strings.TrimSpace(*item2.DefaultModelRef) == "" {
			if s.defModel != "" {
				v := s.defModel
				dm = &v
			}
		}
		if item2.DefaultEmbedderRef == nil || strings.TrimSpace(*item2.DefaultEmbedderRef) == "" {
			if s.defEmbed != "" {
				v := s.defEmbed
				de = &v
			}
		}
		if da != nil || dm != nil || de != nil {
			_ = s.users.UpdatePreferencesByUsername(r.Context(), username, nil, nil, da, dm, de)
		}
	}
	// store session with tokens (server-side only)
	accessToken := token.AccessToken
	refreshToken := token.RefreshToken
	s.sess.CreateWithTokens(w, username, accessToken, refreshToken, idTokenStr, token.Expiry)
	// Persist encrypted token server-side to allow refresh across restarts
	if s.dao != nil && s.cfg != nil && s.cfg.OAuth != nil && s.cfg.OAuth.Client != nil {
		store := iauth.NewTokenStoreDAO(s.dao, s.cfg.OAuth.Client.ConfigURL)
		t := &iauth.OAuthToken{AccessToken: accessToken, RefreshToken: refreshToken, IDToken: idTokenStr, ExpiresAt: token.Expiry}
		prov := strings.TrimSpace(s.cfg.OAuth.Name)
		if prov == "" {
			prov = "oauth"
		}
		if err := store.Upsert(r.Context(), id, prov, t); err != nil {
			encode(w, http.StatusInternalServerError, nil, err)
			return
		}
	}
	// Post a tiny HTML that closes the popup
	w.Header().Set("Content-Type", "text/html")
	_, _ = w.Write([]byte(`<html><body><script>if (window.opener) { try { window.opener.postMessage({type:'oauth',status:'ok'}, '*'); } catch(e){} } window.close();</script>OK</body></html>`))
}

// Helpers for BFF state
var bfCipher = blowfish.Cipher{}

func (s *Service) encryptState(ctx context.Context, codeVerifier string) string {
	salt := s.cfg.OAuth.Client.ConfigURL
	key := &kms.Key{Kind: "raw", Raw: string(blowfish.EnsureKey([]byte(salt)))}
	b, _ := bfCipher.Encrypt(ctx, key, []byte(codeVerifier))
	return base64RawURL(b)
}

func (s *Service) decryptState(ctx context.Context, state string) (string, error) {
	salt := s.cfg.OAuth.Client.ConfigURL
	key := &kms.Key{Kind: "raw", Raw: string(blowfish.EnsureKey([]byte(salt)))}
	raw, err := base64RawURLDecode(state)
	if err != nil {
		return "", err
	}
	buf, err := bfCipher.Decrypt(ctx, key, raw)
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

func (s *Service) callbackURL(r *http.Request) string {
	// Build absolute callback URL: scheme + host from headers; default to http if missing
	scheme := r.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	path := s.cfg.RedirectPath
	if strings.TrimSpace(path) == "" {
		path = "/v1/api/auth/oauth/callback"
	}
	return scheme + "://" + host + path
}

// minimal base64 url helpers
func base64RawURL(b []byte) string {
	return strings.TrimRight(base64.URLEncoding.EncodeToString(b), "=")
}
func base64RawURLDecode(s string) ([]byte, error) {
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	return base64.URLEncoding.DecodeString(s)
}

// buildVerifierFromConfig mirrors middleware initialization for BFF callback validation
func buildVerifierFromConfig(ctx context.Context, cfg *iauth.Config) *vcfg.Service {
	var v *vcfg.Service
	if cfg != nil && cfg.OAuth != nil && cfg.OAuth.Client != nil {
		if jwks := strings.TrimSpace(cfg.OAuth.Client.JWKSURL); jwks != "" {
			cand := vcfg.New(&vcfg.Config{CertURL: jwks})
			if err := cand.Init(ctx); err == nil {
				v = cand
			}
		}
		if v == nil && strings.TrimSpace(cfg.OAuth.Client.DiscoveryURL) != "" {
			if jwks, err := iauth.JWKSFromDiscovery(ctx, cfg.OAuth.Client.DiscoveryURL); err == nil && strings.TrimSpace(jwks) != "" {
				cand := vcfg.New(&vcfg.Config{CertURL: jwks})
				if err := cand.Init(ctx); err == nil {
					v = cand
				}
			}
		}
		if v == nil && strings.TrimSpace(cfg.OAuth.Client.ConfigURL) != "" {
			if jwks, err := iauth.JWKSFromBFFConfig(ctx, cfg.OAuth.Client.ConfigURL); err == nil && strings.TrimSpace(jwks) != "" {
				cand := vcfg.New(&vcfg.Config{CertURL: jwks})
				if err := cand.Init(ctx); err == nil {
					v = cand
				}
			}
		}
	}
	return v
}

// verifyIDToken validates the id_token signature and optional iss/aud checks,
// and extracts common identity fields (sub/email/name). It returns a friendly error
// suitable for API responses.
func (s *Service) verifyIDToken(ctx context.Context, idToken string) (sub, email, name string, err error) {
	verifier := buildVerifierFromConfig(ctx, s.cfg)
	if verifier == nil {
		return "", "", "", errf("oidc verifier not configured")
	}
	claims, vErr := verifier.VerifyClaims(ctx, idToken)
	if vErr != nil {
		return "", "", "", errf("id token verification failed")
	}
	// Optional iss/aud checks
	if s.cfg != nil && s.cfg.OAuth != nil && s.cfg.OAuth.Client != nil {
		iss := strings.TrimSpace(s.cfg.OAuth.Client.Issuer)
		if iss != "" && strings.TrimSpace(claims.Issuer) != iss {
			return "", "", "", errf("issuer mismatch")
		}
		if len(s.cfg.OAuth.Client.Audiences) > 0 {
			ok := false
			for _, aud := range s.cfg.OAuth.Client.Audiences {
				if claims.VerifyAudience(strings.TrimSpace(aud), true) {
					ok = true
					break
				}
			}
			if !ok {
				return "", "", "", errf("audience mismatch")
			}
		}
	}
	sub = claims.Subject
	email = claims.Email
	name = claims.Username
	if strings.TrimSpace(name) == "" {
		// Fallback: parse unverified for 'name' field if present
		if t, _, perr := new(jwt.Parser).ParseUnverified(idToken, jwt.MapClaims{}); perr == nil {
			if mc, ok := t.Claims.(jwt.MapClaims); ok {
				if v, ok := mc["name"].(string); ok {
					name = v
				}
			}
		}
	}
	return sub, email, name, nil
}

// handleProviders returns a list of configured auth providers based on mode and environment.
func (s *Service) handleProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	type provider struct {
		Name            string `json:"name"`
		Label           string `json:"label"`
		Type            string `json:"type"` // local|bff|oidc
		DefaultUsername string `json:"defaultUsername,omitempty"`
		// SPA public client metadata (non-secret)
		ClientID     string   `json:"clientID,omitempty"`
		DiscoveryURL string   `json:"discoveryURL,omitempty"`
		RedirectURI  string   `json:"redirectURI,omitempty"`
		Scopes       []string `json:"scopes,omitempty"`
	}
	var items []provider
	letMode := func() string {
		if s.cfg != nil && s.cfg.OAuth != nil {
			return strings.ToLower(strings.TrimSpace(s.cfg.OAuth.Mode))
		}
		return ""
	}
	mode := letMode()
	// Local provider (when allowed)
	localEnabled := s.cfg != nil && s.cfg.Local != nil && s.cfg.Local.Enabled
	if localEnabled {
		p := provider{Name: "local", Label: "Local User", Type: "local"}
		if s.cfg != nil && strings.TrimSpace(s.cfg.DefaultUsername) != "" {
			p.DefaultUsername = strings.TrimSpace(s.cfg.DefaultUsername)
		}
		items = append(items, p)
	}
	// BFF provider from workspace config (unified oauth)
	if (mode == "bff" || mode == "mixed") && s.cfg != nil && s.cfg.OAuth != nil && s.cfg.OAuth.Client != nil {
		if strings.TrimSpace(s.cfg.OAuth.Client.ConfigURL) != "" {
			name := s.cfg.OAuth.Name
			if name == "" {
				name = "bff"
			}
			label := s.cfg.OAuth.Label
			if label == "" {
				label = "OAuth2"
			}
			items = append(items, provider{Name: name, Label: label, Type: "bff"})
		}
	}
	// OIDC (SPA/bearer) provider from workspace config (unified oauth)
	if (mode == "oidc" || mode == "bearer" || mode == "spa" || mode == "mixed") && s.cfg != nil && s.cfg.OAuth != nil && s.cfg.OAuth.Client != nil {
		cli := s.cfg.OAuth.Client
		if strings.TrimSpace(cli.JWKSURL) != "" || strings.TrimSpace(cli.DiscoveryURL) != "" || strings.TrimSpace(cli.ConfigURL) != "" {
			name := s.cfg.OAuth.Name
			if name == "" {
				name = "default"
			}
			label := s.cfg.OAuth.Label
			if label == "" {
				label = "OIDC"
			}
			p := provider{Name: name, Label: label, Type: "oidc"}
			// Expose SPA metadata when present (non-secret)
			if s.cfg.OAuth.Mode == "spa" || s.cfg.OAuth.Mode == "mixed" {
				p.ClientID = strings.TrimSpace(cli.ClientID)
				// Discovery URL: prefer configured; else derive from issuer or confidential client auth domain
				p.DiscoveryURL = strings.TrimSpace(cli.DiscoveryURL)
				if p.DiscoveryURL == "" {
					if iss := strings.TrimSpace(cli.Issuer); iss != "" {
						p.DiscoveryURL = strings.TrimRight(iss, "/") + "/.well-known/openid-configuration"
					}
				}
				if p.DiscoveryURL == "" && strings.TrimSpace(cli.ConfigURL) != "" {
					oa := authorizer.New()
					oc := &authorizer.OAuthConfig{ConfigURL: cli.ConfigURL}
					if err := oa.EnsureConfig(r.Context(), oc); err == nil && oc.Config != nil {
						deriveDiscovery := func(raw string) string {
							u, err := url.Parse(raw)
							if err != nil || u == nil || u.Host == "" {
								return ""
							}
							pth := u.Path
							if strings.Contains(pth, "/oauth2/") {
								parts := strings.Split(pth, "/")
								for i := 0; i < len(parts)-1; i++ {
									if parts[i] == "oauth2" && i+1 < len(parts) {
										base := "/oauth2/" + parts[i+1]
										return u.Scheme + "://" + u.Host + path.Join(base, "/.well-known/openid-configuration")
									}
								}
							}
							if strings.Contains(pth, "/realms/") {
								parts := strings.Split(pth, "/")
								for i := 0; i < len(parts)-1; i++ {
									if parts[i] == "realms" && i+1 < len(parts) {
										base := "/realms/" + parts[i+1]
										return u.Scheme + "://" + u.Host + path.Join(base, "/.well-known/openid-configuration")
									}
								}
							}
							return u.Scheme + "://" + u.Host + "/.well-known/openid-configuration"
						}
						if d := deriveDiscovery(oc.Config.Endpoint.AuthURL); d != "" {
							p.DiscoveryURL = d
						}
						if p.DiscoveryURL == "" {
							if d := deriveDiscovery(oc.Config.Endpoint.TokenURL); d != "" {
								p.DiscoveryURL = d
							}
						}
					}
				}
				// Redirect URI: prefer configured; else default to request origin (SPA handles its own route)
				p.RedirectURI = strings.TrimSpace(cli.RedirectURI)
				{
					constScheme := func() string {
						s := r.Header.Get("X-Forwarded-Proto")
						if s == "" {
							if r.TLS != nil {
								return "https"
							}
							return "http"
						}
						return s
					}
					scheme := constScheme()
					host := r.Header.Get("X-Forwarded-Host")
					if host == "" {
						host = r.Host
					}
					// If empty, default to origin root
					if p.RedirectURI == "" {
						p.RedirectURI = scheme + "://" + host + "/"
					}
					// If relative path, make it absolute on current origin
					if strings.HasPrefix(p.RedirectURI, "/") {
						p.RedirectURI = scheme + "://" + host + p.RedirectURI
					}
				}
				if len(cli.Scopes) > 0 {
					p.Scopes = append(p.Scopes, cli.Scopes...)
				}
			}
			items = append(items, p)
		}
	}
	encode(w, http.StatusOK, items, nil)
}

// Helpers
// reserved for future timeouts
func errf(msg string) error { return &apiErr{msg} }

type apiErr struct{ s string }

func (e *apiErr) Error() string { return e.s }

func value(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
func coalesce(p *string, def string) string {
	if p == nil || strings.TrimSpace(*p) == "" {
		return def
	}
	return *p
}

type apiResponse struct {
	Status  string `json:"status"`
	Data    any    `json:"data,omitempty"`
	Message string `json:"message,omitempty"`
}

func encode(w http.ResponseWriter, code int, data any, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		if code == 0 {
			code = http.StatusInternalServerError
		}
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(apiResponse{Status: "ERROR", Message: err.Error()})
		return
	}
	if code == 0 {
		code = http.StatusOK
	}
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(apiResponse{Status: "ok", Data: data})
}
