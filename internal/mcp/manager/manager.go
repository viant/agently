package manager

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	authctx "github.com/viant/agently/internal/auth"
	mcpcfg "github.com/viant/agently/internal/mcp/config"
	"github.com/viant/mcp"
	protoclient "github.com/viant/mcp-protocol/client"
	mcpclient "github.com/viant/mcp/client"
	auth "github.com/viant/mcp/client/auth"
	authtransport "github.com/viant/mcp/client/auth/transport"
)

// Provider returns client options for a given MCP server name.
type Provider interface {
	Options(ctx context.Context, serverName string) (*mcpcfg.MCPClient, error)
}

// Option configures Manager. It can return an error which will be bubbled up by New.
type Option func(*Manager) error

// WithTTL sets idle TTL before reaping a client.
func WithTTL(ttl time.Duration) Option { return func(m *Manager) error { m.ttl = ttl; return nil } }

// WithHandlerFactory sets a factory for per-connection client handlers (for elicitation, etc.).
func WithHandlerFactory(newHandler func() protoclient.Handler) Option {
	return func(m *Manager) error { m.newHandler = newHandler; return nil }
}

// WithCookieJar injects a host-controlled CookieJar that will be applied to
// newly created MCP clients via ClientOptions, overriding any per-provider jar.
func WithCookieJar(jar http.CookieJar) Option {
	return func(m *Manager) error { m.cookieJar = jar; return nil }
}

// JarProvider returns a per-request CookieJar (e.g., per-user) chosen from context.
// When provided, it takes precedence over the static cookieJar set via WithCookieJar.
type JarProvider func(ctx context.Context) (http.CookieJar, error)

// WithCookieJarProvider injects a provider that selects a CookieJar per request (e.g., per user).
func WithCookieJarProvider(p JarProvider) Option {
	return func(m *Manager) error { m.jarProvider = p; return nil }
}

// WithAuthRoundTripper enables auth integration by attaching the provided
// RoundTripper as an Authorizer interceptor to created MCP clients.
func WithAuthRoundTripper(rt *authtransport.RoundTripper) Option {
	return func(m *Manager) error { m.authRT = rt; return nil }
}

// AuthRTProvider returns a per-request auth RoundTripper (e.g., per-user) chosen from context.
// When provided, it takes precedence over the static authRT set via WithAuthRoundTripper.
type AuthRTProvider func(ctx context.Context) *authtransport.RoundTripper

// WithAuthRoundTripperProvider injects a provider that selects an auth RoundTripper per request.
func WithAuthRoundTripperProvider(p AuthRTProvider) Option {
	return func(m *Manager) error { m.authRTProvider = p; return nil }
}

// Manager caches MCP clients per (conversationID, serverName) and handles idle reaping.
type Manager struct {
	prov           Provider
	ttl            time.Duration
	newHandler     func() protoclient.Handler
	cookieJar      http.CookieJar
	jarProvider    JarProvider
	authRT         *authtransport.RoundTripper
	authRTProvider AuthRTProvider
	anonJar        http.CookieJar

	mu   sync.Mutex
	pool map[string]map[string]*entry // convID -> serverName -> entry
}

type entry struct {
	client mcpclient.Interface
	usedAt time.Time
}

// New creates a Manager with the given Provider and options.
func New(prov Provider, opts ...Option) (*Manager, error) {
	m := &Manager{prov: prov, ttl: 30 * time.Minute, pool: map[string]map[string]*entry{}}
	for _, o := range opts {
		if err := o(m); err != nil {
			return nil, fmt.Errorf("mcp manager option: %w", err)
		}
	}
	return m, nil
}

// Options exposes the underlying provider client options (authoring metadata,
// timeouts, etc.) for a given server name.
func (m *Manager) Options(ctx context.Context, serverName string) (*mcpcfg.MCPClient, error) {
	if m == nil || m.prov == nil {
		return nil, errors.New("mcp manager: provider not configured")
	}
	return m.prov.Options(ctx, serverName)
}

// Get returns an MCP client for (convID, serverName), creating it if needed.
func (m *Manager) Get(ctx context.Context, convID, serverName string) (mcpclient.Interface, error) {
	if m.prov == nil {
		return nil, errors.New("mcp manager: provider not configured")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	// Maintain per-conversation client to correlate elicitation correctly.
	if m.pool[convID] == nil {
		m.pool[convID] = map[string]*entry{}
	}
	if e := m.pool[convID][serverName]; e != nil && e.client != nil {
		e.usedAt = time.Now()
		return e.client, nil
	}
	opts, err := m.prov.Options(ctx, serverName)
	if err != nil {
		return nil, err
	}
	if opts == nil {
		return nil, errors.New("mcp manager: nil client options")
	}
	opts.Init()
	// Select per-request jar (provider beats static) and merge provider cookies.json into it
	// (if both present) before override,
	// so the very first POST can carry previously minted session cookies.
	var effectiveJar http.CookieJar
	if m.jarProvider != nil {
		var jerr error
		effectiveJar, jerr = m.jarProvider(ctx)
		if jerr != nil {
			return nil, fmt.Errorf("cookie jar provider: %w", jerr)
		}
	} else {
		effectiveJar = m.cookieJar
	}
	if effectiveJar != nil && opts.ClientOptions != nil {
		// Determine origin from transport URL
		origin := strings.TrimSpace(opts.ClientOptions.Transport.URL)
		if origin != "" {
			if u, perr := url.Parse(origin); perr == nil {
				// If we have an anonymous jar and current request is for a named user,
				// merge cookies from anonymous into the user's jar for this origin.
				userID := strings.TrimSpace(authctx.EffectiveUserID(ctx))
				if userID != "" && m.anonJar != nil && m.anonJar != effectiveJar {
					if cs := m.anonJar.Cookies(u); len(cs) > 0 {
						effectiveJar.SetCookies(u, cs)
					}
				}
				if pj := opts.ClientOptions.CookieJar; pj != nil && pj != effectiveJar {
					if cs := pj.Cookies(u); len(cs) > 0 {
						effectiveJar.SetCookies(u, cs)
					}
				}
			}
		}
		// Override CookieJar with selected jar to ensure reuse across conversations
		opts.ClientOptions.CookieJar = effectiveJar
		// Track anonymous jar to support later migration when identity becomes available
		if strings.TrimSpace(authctx.EffectiveUserID(ctx)) == "" {
			m.anonJar = effectiveJar
		}
	}
	handler := m.newHandler
	if handler == nil {
		handler = func() protoclient.Handler { return nil }
	}
	h := handler()
	// If handler supports setting conversation id, assign it.
	if ca, ok := h.(interface{ SetConversationID(string) }); ok {
		ca.SetConversationID(convID)
	}
	cli, err := mcp.NewClient(h, opts.ClientOptions)
	if err != nil {
		return nil, err
	}
	// Attach auth interceptor when configured (prefer per-request provider)
	var rt *authtransport.RoundTripper
	if m.authRTProvider != nil {
		rt = m.authRTProvider(ctx)
	}
	if rt == nil {
		rt = m.authRT
	}
	if rt != nil {
		authorizer := auth.NewAuthorizer(rt)
		// apply option to concrete client
		mcpclient.WithAuthInterceptor(authorizer)(cli)
	}
	ent := &entry{client: cli, usedAt: time.Now()}
	m.pool[convID][serverName] = ent
	return cli, nil
}

// Touch updates last-used time for (convID, serverName).
func (m *Manager) Touch(convID, serverName string) {
	m.mu.Lock()
	if e := m.pool[convID][serverName]; e != nil {
		e.usedAt = time.Now()
	}
	m.mu.Unlock()
}

// CloseConversation drops all clients for a conversation.
// Note: underlying transports may keep connections if the library doesn't expose Close.
func (m *Manager) CloseConversation(convID string) {
	m.mu.Lock()
	delete(m.pool, convID)
	m.mu.Unlock()
}

// Reap closes idle clients beyond TTL by dropping references.
func (m *Manager) Reap() {
	cutoff := time.Now().Add(-m.ttl)
	m.mu.Lock()
	for convID, perServer := range m.pool {
		for server, e := range perServer {
			if e == nil || e.usedAt.Before(cutoff) {
				delete(perServer, server)
			}
		}
		if len(perServer) == 0 {
			delete(m.pool, convID)
		}
	}
	m.mu.Unlock()
}

// Reconnect drops the cached client for (convID, serverName) and creates a new one.
// It returns the fresh client or an error if recreation fails.
func (m *Manager) Reconnect(ctx context.Context, convID, serverName string) (mcpclient.Interface, error) {
	if m == nil {
		return nil, errors.New("mcp manager: nil manager")
	}
	// Drop existing entry to force re-creation
	m.mu.Lock()
	if m.pool[convID] != nil {
		delete(m.pool[convID], serverName)
		if len(m.pool[convID]) == 0 {
			delete(m.pool, convID)
		}
	}
	m.mu.Unlock()
	// Recreate
	return m.Get(ctx, convID, serverName)
}

// StartReaper launches a background goroutine that periodically invokes Reap
// until the provided context is cancelled or the returned stop function is
// called. If interval is non-positive, ttl/2 is used with a minimum of 1 minute.
func (m *Manager) StartReaper(ctx context.Context, interval time.Duration) (stop func()) {
	min := time.Minute
	if interval <= 0 {
		interval = m.ttl / 2
		if interval < min {
			interval = min
		}
	}
	done := make(chan struct{})
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				m.Reap()
			case <-ctx.Done():
				return
			case <-done:
				return
			}
		}
	}()
	return func() { close(done) }
}
