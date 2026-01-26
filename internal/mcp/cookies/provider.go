package cookies

import (
	"context"
	"fmt"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/viant/afs"
	authctx "github.com/viant/agently/internal/auth"
	"github.com/viant/agently/internal/workspace"
	mcprepo "github.com/viant/agently/internal/workspace/repository/mcp"
	authtransport "github.com/viant/mcp/client/auth/transport"
)

// Provider returns per-user cookie jars used by MCP HTTP clients (SSE/streamable)
// and auth transports.
//
// The jar is persisted under:
//
//	$AGENTLY_STATE_PATH/mcp/bff/<user>/cookies.json
//
// It is optionally "warmed" from provider-specific jars:
//
//	$AGENTLY_STATE_PATH/mcp/<server>/<user>/cookies.json
//
// By default, cookies are never migrated from anonymous scopes into a named user
// to avoid accidental cross-identity reuse.
type Provider struct {
	fs      afs.Service
	mcpRepo *mcprepo.Repository

	mu     sync.Mutex
	byUser map[string]http.CookieJar

	includeAnonymousScope bool
}

// Option customises Provider behavior.
type Option func(*Provider)

// WithAnonymousScope enables warming a named user jar with cookies from the
// "anonymous" scope (legacy behavior). Default is disabled.
func WithAnonymousScope(enabled bool) Option {
	return func(p *Provider) { p.includeAnonymousScope = enabled }
}

// New returns a Provider backed by filesystem persistence.
func New(fs afs.Service, repo *mcprepo.Repository, options ...Option) *Provider {
	if fs == nil {
		fs = afs.New()
	}
	if repo == nil {
		repo = mcprepo.New(fs)
	}
	p := &Provider{
		fs:      fs,
		mcpRepo: repo,
		byUser:  map[string]http.CookieJar{},
	}
	for _, opt := range options {
		if opt != nil {
			opt(p)
		}
	}
	return p
}

// Jar returns a cached per-user cookie jar.
func (p *Provider) Jar(ctx context.Context) (http.CookieJar, error) {
	user := strings.TrimSpace(authctx.EffectiveUserID(ctx))
	if user == "" {
		user = "anonymous"
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if existing := p.byUser[user]; existing != nil {
		return existing, nil
	}

	sharedDir := filepath.Join(workspace.StateRoot(), "mcp", "bff", user)
	sharedPath := filepath.Join(sharedDir, "cookies.json")
	if err := os.MkdirAll(sharedDir, 0o700); err != nil {
		return nil, err
	}
	jar, err := authtransport.NewFileJar(sharedPath)
	if err != nil {
		return nil, err
	}

	if err := p.warmProviderCookies(jar, user); err != nil {
		return nil, err
	}

	p.byUser[user] = jar
	return jar, nil
}

func (p *Provider) warmProviderCookies(dst http.CookieJar, user string) error {
	names, err := p.mcpRepo.List(context.Background())
	if err != nil {
		return err
	}

	scopes := []string{user}
	if user == "anonymous" {
		// No cross-scope migration needed.
	} else if p.includeAnonymousScope {
		scopes = append(scopes, "anonymous")
	}

	for _, name := range names {
		cfg, err := p.mcpRepo.Load(context.Background(), name)
		if err != nil || cfg == nil || cfg.ClientOptions == nil {
			continue
		}
		raw := strings.TrimSpace(cfg.ClientOptions.Transport.URL)
		if raw == "" {
			continue
		}
		u, perr := neturl.Parse(raw)
		if perr != nil {
			continue
		}
		for _, scope := range scopes {
			stateDir := filepath.Join(workspace.StateRoot(), "mcp", name, scope)
			cookiesPath := filepath.Join(stateDir, "cookies.json")
			ok, _ := p.fs.Exists(context.Background(), cookiesPath)
			if !ok {
				continue
			}
			src, jerr := authtransport.NewFileJar(cookiesPath)
			if jerr != nil || src == nil {
				continue
			}
			if cs := src.Cookies(u); len(cs) > 0 {
				dst.SetCookies(u, cs)
				mirrorDevAliasCookies(dst, u, cs)
			}
		}
	}
	return nil
}

func mirrorDevAliasCookies(dst http.CookieJar, u *neturl.URL, cs []*http.Cookie) {
	host, port := u.Hostname(), u.Port()
	var alt string
	if host == "localhost" {
		alt = "127.0.0.1"
	} else if host == "127.0.0.1" {
		alt = "localhost"
	}
	if alt == "" {
		return
	}
	altURL := *u
	if port != "" {
		altURL.Host = alt + ":" + port
	} else {
		altURL.Host = alt
	}
	dst.SetCookies(&altURL, cs)
}

func (p *Provider) String() string {
	return fmt.Sprintf("cookies.Provider(anonymousScope=%v)", p.includeAnonymousScope)
}
