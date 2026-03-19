package v1

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/viant/agently-core/app/executor"
	svcauth "github.com/viant/agently-core/service/auth"
	convdao "github.com/viant/agently/internal/service/conversation"
	sessionread "github.com/viant/agently/pkg/agently/user/session"
	sessiondelete "github.com/viant/agently/pkg/agently/user/session/delete"
	sessionwrite "github.com/viant/agently/pkg/agently/user/session/write"
	"github.com/viant/scy"
	scyauth "github.com/viant/scy/auth"
	vcfg "github.com/viant/scy/auth/jwt/verifier"
	"gopkg.in/yaml.v3"
)

type authRuntime struct {
	cfg         *authConfig
	sessions    *svcauth.Manager
	jwtMintKey  string
	jwtVerifier *vcfg.Service
	handlerOpts []svcauth.HandlerOption
	ext         *authExtension
	stopRefresh func() // stops the background token refresh goroutine
}

func withAuthExtensions(base http.Handler, runtime *authRuntime) http.Handler {
	if runtime == nil || runtime.ext == nil {
		return base
	}
	mux := http.NewServeMux()
	runtime.ext.Register(mux)
	mux.Handle("/", base)
	return runtime.protect(mux)
}

func newAuthRuntime(ctx context.Context, workspaceRoot string, rt *executor.Runtime) (*authRuntime, error) {
	cfg, err := loadWorkspaceAuthConfig(workspaceRoot)
	if err != nil {
		return nil, err
	}
	if cfg == nil || !cfg.Enabled {
		return nil, nil
	}

	if strings.TrimSpace(cfg.CookieName) == "" {
		cfg.CookieName = "agently_session"
	}
	if cfg.SessionTTLHours <= 0 {
		cfg.SessionTTLHours = 24 * 7
	}
	if strings.TrimSpace(cfg.RedirectPath) == "" {
		cfg.RedirectPath = "/v1/api/auth/oauth/callback"
	}
	if strings.TrimSpace(cfg.IpHashKey) == "" {
		cfg.IpHashKey = "agently-app-dev-ip-hash-key"
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid auth configuration: %w", err)
	}

	// Obtain the shared Datly DAO (same singleton as the original agently router)
	// and register session CRUD routes, matching router.go:133-153.
	dao, err := convdao.NewDatly(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize datly for session persistence: %w", err)
	}
	var sessionStore svcauth.SessionStore
	if dao != nil {
		if err := sessionread.DefineSessionComponent(ctx, dao); err != nil {
			return nil, fmt.Errorf("failed to register session read component: %w", err)
		}
		if _, err := sessiondelete.DefineComponent(ctx, dao); err != nil {
			return nil, fmt.Errorf("failed to register session delete component: %w", err)
		}
		if _, err := sessionwrite.DefineComponent(ctx, dao); err != nil {
			return nil, fmt.Errorf("failed to register session write component: %w", err)
		}
		sessionStore = newSessionStoreAdapter(dao)
	}
	sessions := svcauth.NewManager(time.Duration(cfg.SessionTTLHours)*time.Hour, sessionStore)
	opts := make([]svcauth.HandlerOption, 0, 2)

	var tokenStore svcauth.TokenStore
	if dao != nil && cfg.OAuth != nil && cfg.OAuth.Client != nil {
		if configURL := strings.TrimSpace(cfg.OAuth.Client.ConfigURL); configURL != "" {
			tokenStore = svcauth.NewTokenStoreDAO(dao, configURL)
			opts = append(opts, svcauth.WithTokenStore(tokenStore))
		}
	}

	var jwtVerifier *vcfg.Service
	if cfg.JWT != nil && cfg.JWT.Enabled {
		verifyCfg := &vcfg.Config{CertURL: strings.TrimSpace(cfg.JWT.CertURL)}
		for _, rsaPath := range cfg.JWT.RSA {
			trimmed := strings.TrimSpace(rsaPath)
			if trimmed == "" {
				continue
			}
			verifyCfg.RSA = append(verifyCfg.RSA, scy.NewResource("", trimmed, ""))
		}
		if hmac := strings.TrimSpace(cfg.JWT.HMAC); hmac != "" {
			verifyCfg.HMAC = scy.NewResource("", hmac, "")
		}
		v := vcfg.New(verifyCfg)
		if err := v.Init(ctx); err != nil {
			return nil, fmt.Errorf("unable to initialize jwt verifier: %w", err)
		}
		jwtVerifier = v
	}

	ar := &authRuntime{
		cfg:         cfg,
		sessions:    sessions,
		jwtMintKey:  strings.TrimSpace(jwtPrivateKeyPath(cfg)),
		jwtVerifier: jwtVerifier,
		handlerOpts: opts,
		ext:         newAuthExtension(cfg, sessions, strings.TrimSpace(jwtPrivateKeyPath(cfg)), tokenStore),
	}
	// Start background token refresh watcher.
	ar.stopRefresh = ar.startTokenRefreshWatcher(ctx)
	return ar, nil
}

type workspaceAuthConfig struct {
	Auth *authConfig `yaml:"auth"`
}

func loadWorkspaceAuthConfig(workspaceRoot string) (*authConfig, error) {
	root := strings.TrimSpace(workspaceRoot)
	if root == "" {
		return nil, nil
	}
	cfgPath := filepath.Join(root, "config.yaml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("unable to read workspace config: %w", err)
	}
	cfg := &workspaceAuthConfig{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("unable to parse workspace auth config: %w", err)
	}
	if cfg.Auth == nil {
		return nil, nil
	}
	return cfg.Auth, nil
}

type authConfig struct {
	Enabled         bool           `yaml:"enabled" json:"enabled"`
	CookieName      string         `yaml:"cookieName" json:"cookieName"`
	SessionTTLHours int            `yaml:"sessionTTLHours,omitempty" json:"sessionTTLHours,omitempty"`
	DefaultUsername string         `yaml:"defaultUsername" json:"defaultUsername"`
	IpHashKey       string         `yaml:"ipHashKey" json:"ipHashKey"`
	TrustedProxies  []string       `yaml:"trustedProxies" json:"trustedProxies"`
	RedirectPath    string         `yaml:"redirectPath" json:"redirectPath"`
	OAuth           *oauthConfig   `yaml:"oauth" json:"oauth"`
	Local           *localConfig   `yaml:"local" json:"local"`
	JWT             *jwtAuthConfig `yaml:"jwt,omitempty" json:"jwt,omitempty"`
}

type oauthConfig struct {
	Mode   string       `yaml:"mode" json:"mode"`
	Name   string       `yaml:"name" json:"name"`
	Label  string       `yaml:"label" json:"label"`
	Client *oauthClient `yaml:"client" json:"client"`
}

type oauthClient struct {
	ConfigURL    string   `yaml:"configURL" json:"configURL"`
	DiscoveryURL string   `yaml:"discoveryURL" json:"discoveryURL"`
	JWKSURL      string   `yaml:"jwksURL" json:"jwksURL"`
	RedirectURI  string   `yaml:"redirectURI" json:"redirectURI"`
	ClientID     string   `yaml:"clientID" json:"clientID"`
	Scopes       []string `yaml:"scopes" json:"scopes"`
	Issuer       string   `yaml:"issuer" json:"issuer"`
	Audiences    []string `yaml:"audiences" json:"audiences"`
}

type localConfig struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
}

type jwtAuthConfig struct {
	Enabled       bool     `yaml:"enabled" json:"enabled"`
	RSA           []string `yaml:"rsa,omitempty" json:"rsa,omitempty"`
	HMAC          string   `yaml:"hmac,omitempty" json:"hmac,omitempty"`
	CertURL       string   `yaml:"certURL,omitempty" json:"certURL,omitempty"`
	RSAPrivateKey string   `yaml:"rsaPrivateKey,omitempty" json:"rsaPrivateKey,omitempty"`
}

func (c *authConfig) Validate() error {
	if c == nil || !c.Enabled {
		return nil
	}
	if strings.TrimSpace(c.IpHashKey) == "" {
		return fmt.Errorf("auth.ipHashKey is required when auth is enabled")
	}
	needsCookie := c.Local != nil && c.Local.Enabled
	if c.OAuth != nil {
		mode := strings.ToLower(strings.TrimSpace(c.OAuth.Mode))
		if mode == "bff" || mode == "mixed" || mode == "local" {
			needsCookie = true
		}
	}
	if needsCookie && strings.TrimSpace(c.CookieName) == "" {
		return fmt.Errorf("auth.cookieName is required for cookie-based auth")
	}
	return nil
}

func jwtPrivateKeyPath(cfg *authConfig) string {
	if cfg == nil || cfg.JWT == nil || !cfg.JWT.Enabled {
		return ""
	}
	return strings.TrimSpace(cfg.JWT.RSAPrivateKey)
}

type authContextKey struct{}

type authUser struct {
	Subject string
	Email   string
	Tokens  *scyauth.Token // OAuth tokens from session (for MCP token forwarding)
}

func withAuthUser(ctx context.Context, user *authUser) context.Context {
	if user == nil {
		return ctx
	}
	ctx = context.WithValue(ctx, authContextKey{}, *user)
	// Also inject into agently-core's auth context so EffectiveUserID works for scheduler
	ctx = svcauth.InjectUser(ctx, user.Subject)
	// Inject OAuth tokens so MCP clients can forward them (via MCPAuthToken).
	if user.Tokens != nil {
		ctx = svcauth.InjectTokens(ctx, user.Tokens)
	}
	return ctx
}

func authUserFromContext(ctx context.Context) *authUser {
	if ctx == nil {
		return nil
	}
	if raw, ok := ctx.Value(authContextKey{}).(authUser); ok {
		return &raw
	}
	return nil
}

func (a *authRuntime) protect(next http.Handler) http.Handler {
	if a == nil || a.cfg == nil || !a.cfg.Enabled {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions || r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		user := a.authenticate(r)
		if user == nil {
			user = a.ensureDefaultUser(w, r)
		}
		ctx := r.Context()
		if user != nil {
			ctx = withAuthUser(ctx, user)
		}
		if strings.HasPrefix(r.URL.Path, "/v1/api/auth/") {
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/v1/") {
			if user == nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"status":"error","message":"authorization required"}`))
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *authRuntime) authenticate(r *http.Request) *authUser {
	if a == nil || r == nil {
		return nil
	}
	authz := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(authz), "bearer ") && a.jwtVerifier != nil {
		token := strings.TrimSpace(authz[len("Bearer "):])
		if token == "" {
			return nil
		}
		claims, err := a.jwtVerifier.VerifyClaims(r.Context(), token)
		if err == nil && claims != nil {
			tok := &scyauth.Token{}
			tok.Token.AccessToken = token
			return &authUser{
				Subject: strings.TrimSpace(firstNonEmpty(claims.Subject, claims.Username)),
				Email:   strings.TrimSpace(claims.Email),
				Tokens:  tok,
			}
		}
	}
	if a.sessions != nil && strings.TrimSpace(a.cfg.CookieName) != "" {
		if c, err := r.Cookie(a.cfg.CookieName); err == nil && strings.TrimSpace(c.Value) != "" {
			if sess := a.sessions.Get(r.Context(), strings.TrimSpace(c.Value)); sess != nil {
				// If tokens exist but are expired, try inline refresh as last resort.
				if sess.Tokens != nil && !sess.Tokens.Expiry.IsZero() && !sess.Tokens.Valid() {
					refreshed := a.tryRefreshToken(r.Context(), sess)
					if refreshed != nil {
						sess.Tokens = refreshed
					} else {
						// Refresh failed — invalidate session, force re-login.
						log.Printf("[v1-auth] token expired and refresh failed, invalidating session user=%q", sess.Subject)
						a.sessions.Delete(r.Context(), c.Value)
						return nil
					}
				}
				return &authUser{
					Subject: strings.TrimSpace(firstNonEmpty(sess.Subject, sess.Username)),
					Email:   strings.TrimSpace(sess.Email),
					Tokens:  sess.Tokens,
				}
			}
		}
	}
	return nil
}

// tryRefreshToken attempts to refresh an expired OAuth token using the refresh token.
// Returns new token bundle or nil on failure.
// workerID identifies this process for distributed lease ownership.
var workerID = func() string {
	host, _ := os.Hostname()
	if host == "" {
		host = "unknown"
	}
	return fmt.Sprintf("%s:%d", host, os.Getpid())
}()

// tryRefreshToken attempts to refresh an expired OAuth token using the token
// store's distributed lease to prevent multi-node races.
func (a *authRuntime) tryRefreshToken(ctx context.Context, sess *svcauth.Session) *scyauth.Token {
	if sess == nil || sess.Tokens == nil || sess.Tokens.RefreshToken == "" {
		return nil
	}
	if a.ext == nil || a.ext.cfg == nil || a.ext.cfg.OAuth == nil || a.ext.cfg.OAuth.Client == nil {
		return nil
	}
	username := strings.TrimSpace(sess.Subject)
	provider := a.ext.oauthProviderName()

	// Acquire distributed refresh lease via token store (prevents multi-node race).
	tokenStore := a.ext.tokenStore
	if tokenStore != nil {
		_, acquired, err := tokenStore.TryAcquireRefreshLease(ctx, username, provider, workerID, 30*time.Second)
		if err != nil {
			log.Printf("[token-refresh] lease acquire error user=%q err=%v", username, err)
			return nil
		}
		if !acquired {
			// Another node is refreshing — skip, it will update the store.
			return nil
		}
		defer func() {
			_ = tokenStore.ReleaseRefreshLease(ctx, username, provider, workerID)
		}()
	}

	oauthCfg, _ := loadOAuthClientConfig(ctx, a.ext.cfg.OAuth.Client.ConfigURL)
	if oauthCfg == nil {
		return nil
	}
	ts := oauthCfg.TokenSource(ctx, &sess.Tokens.Token)
	refreshed, err := ts.Token()
	if err != nil {
		log.Printf("[token-refresh] failed user=%q err=%v", username, err)
		return nil
	}
	if refreshed.RefreshToken == "" {
		refreshed.RefreshToken = sess.Tokens.RefreshToken
	}
	result := &scyauth.Token{Token: *refreshed, IDToken: sess.Tokens.IDToken}

	// Update session in memory.
	sess.Tokens = result
	a.sessions.Put(ctx, sess)

	// Update token store (DB) so other nodes see the fresh token.
	if tokenStore != nil {
		_ = tokenStore.Put(ctx, &svcauth.OAuthToken{
			Username:     username,
			Provider:     provider,
			AccessToken:  refreshed.AccessToken,
			IDToken:      sess.Tokens.IDToken,
			RefreshToken: refreshed.RefreshToken,
			ExpiresAt:    refreshed.Expiry,
		})
	}
	log.Printf("[token-refresh] ok user=%q newExpiry=%v", username, refreshed.Expiry.Format(time.RFC3339))
	return result
}

// startTokenRefreshWatcher launches a background goroutine that periodically
// scans active sessions and refreshes OAuth tokens before they expire.
// Returns a stop function.
func (a *authRuntime) startTokenRefreshWatcher(ctx context.Context) func() {
	if a == nil || a.sessions == nil {
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				a.refreshExpiringSessions(ctx)
			case <-done:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	return func() { close(done) }
}

// refreshExpiringSessions iterates active sessions and refreshes tokens that
// are expired or will expire within the next 2 minutes.
func (a *authRuntime) refreshExpiringSessions(ctx context.Context) {
	if a == nil || a.sessions == nil {
		return
	}
	sessions := a.sessions.ActiveSessions()
	if len(sessions) == 0 {
		return
	}
	horizon := time.Now().Add(2 * time.Minute)
	var checked, refreshed int
	for _, sess := range sessions {
		if sess == nil || sess.Tokens == nil || sess.Tokens.RefreshToken == "" {
			continue
		}
		checked++
		// Skip tokens that are still valid beyond the horizon.
		if !sess.Tokens.Expiry.IsZero() && sess.Tokens.Expiry.After(horizon) {
			continue
		}
		if a.tryRefreshToken(ctx, sess) != nil {
			refreshed++
		}
	}
	if checked > 0 {
		log.Printf("[token-watcher] sessions=%d checked=%d refreshed=%d", len(sessions), checked, refreshed)
	}
}

func (a *authRuntime) ensureDefaultUser(w http.ResponseWriter, r *http.Request) *authUser {
	if a == nil || a.sessions == nil || a.cfg == nil {
		return nil
	}
	username := strings.TrimSpace(a.cfg.DefaultUsername)
	if username == "" {
		return nil
	}
	session := &svcauth.Session{
		ID:        fmt.Sprintf("auto-%d", time.Now().UnixNano()),
		Username:  username,
		Subject:   username,
		CreatedAt: time.Now(),
	}
	a.sessions.Put(r.Context(), session)
	writeSessionCookie(w, a.cfg, a.sessions, session.ID)
	return &authUser{Subject: username}
}
