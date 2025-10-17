package manager

import (
	"context"
	"errors"
	"sync"
	"time"

	mcpcfg "github.com/viant/agently/internal/mcp/config"
	"github.com/viant/mcp"
	protoclient "github.com/viant/mcp-protocol/client"
	mcpclient "github.com/viant/mcp/client"
)

// Provider returns client options for a given MCP server name.
type Provider interface {
	Options(ctx context.Context, serverName string) (*mcpcfg.MCPClient, error)
}

// Option configures Manager.
type Option func(*Manager)

// WithTTL sets idle TTL before reaping a client.
func WithTTL(ttl time.Duration) Option { return func(m *Manager) { m.ttl = ttl } }

// WithHandlerFactory sets a factory for per-connection client handlers (for elicitation, etc.).
func WithHandlerFactory(newHandler func() protoclient.Handler) Option {
	return func(m *Manager) { m.newHandler = newHandler }
}

// Manager caches MCP clients per (conversationID, serverName) and handles idle reaping.
type Manager struct {
	prov       Provider
	ttl        time.Duration
	newHandler func() protoclient.Handler

	mu   sync.Mutex
	pool map[string]map[string]*entry // convID -> serverName -> entry
}

type entry struct {
	client mcpclient.Interface
	usedAt time.Time
}

// New creates a Manager with the given Provider and options.
func New(prov Provider, opts ...Option) *Manager {
	m := &Manager{prov: prov, ttl: 30 * time.Minute, pool: map[string]map[string]*entry{}}
	for _, o := range opts {
		o(m)
	}
	return m
}

// Get returns an MCP client for (convID, serverName), creating it if needed.
func (m *Manager) Get(ctx context.Context, convID, serverName string) (mcpclient.Interface, error) {
	if m.prov == nil {
		return nil, errors.New("mcp manager: provider not configured")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
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
	m.pool[convID][serverName] = &entry{client: cli, usedAt: time.Now()}
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
