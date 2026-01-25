package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Session struct {
	ID        string
	UserID    string
	ExpiresAt time.Time
	// Optional OAuth tokens captured during BFF login. Not exposed to clients.
	AccessToken  string
	RefreshToken string
	IDToken      string
	TokenExpiry  time.Time
}

// SessionStore persists session metadata for reuse across restarts.
type SessionStore interface {
	Get(ctx context.Context, id string) (*SessionRecord, error)
	Upsert(ctx context.Context, rec *SessionRecord) error
	Delete(ctx context.Context, id string) error
}

type Manager struct {
	cfg   *Config
	mu    sync.RWMutex
	mem   map[string]*Session
	ttl   time.Duration
	store SessionStore
}

type ManagerOption func(*Manager)

// WithSessionStore enables persisted session storage (e.g., Datly-backed).
func WithSessionStore(store SessionStore) ManagerOption {
	return func(m *Manager) { m.store = store }
}

func NewManager(cfg *Config, opts ...ManagerOption) *Manager {
	ttl := 7 * 24 * time.Hour
	if cfg != nil && cfg.SessionTTLHours > 0 {
		ttl = time.Duration(cfg.SessionTTLHours) * time.Hour
	}
	m := &Manager{cfg: cfg, mem: map[string]*Session{}, ttl: ttl}
	for _, opt := range opts {
		if opt != nil {
			opt(m)
		}
	}
	return m
}

func (m *Manager) cookie() string {
	name := m.cfg.CookieName
	if name == "" {
		name = "agently_session"
	}
	return name
}

func (m *Manager) randomID() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Create stores a session and sets the cookie.
func (m *Manager) Create(w http.ResponseWriter, userID string) *Session {
	return m.CreateWithProvider(w, userID, m.defaultProvider())
}

// CreateWithProvider stores a session with an explicit provider and sets the cookie.
func (m *Manager) CreateWithProvider(w http.ResponseWriter, userID, provider string) *Session {
	sid := m.randomID()
	s := &Session{ID: sid, UserID: userID, ExpiresAt: time.Now().Add(m.ttl)}
	m.mu.Lock()
	m.mem[sid] = s
	m.mu.Unlock()
	cookie := &http.Cookie{Name: m.cookie(), Value: sid, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode}
	// Persist cookie lifetime explicitly for client (browser) so it survives restarts within TTL
	cookie.Expires = s.ExpiresAt
	cookie.MaxAge = int(m.ttl.Seconds())
	if isTLS(w) {
		cookie.Secure = true
	}
	http.SetCookie(w, cookie)
	m.persist(context.Background(), s, provider)
	return s
}

// CreateWithTokens stores a session with OAuth tokens (BFF) and sets the cookie.
func (m *Manager) CreateWithTokens(w http.ResponseWriter, userID, access, refresh, id string, expiry time.Time) *Session {
	return m.CreateWithTokensProvider(w, userID, m.defaultProvider(), access, refresh, id, expiry)
}

// CreateWithTokensProvider stores a session with OAuth tokens (BFF) and sets the cookie.
func (m *Manager) CreateWithTokensProvider(w http.ResponseWriter, userID, provider, access, refresh, id string, expiry time.Time) *Session {
	sid := m.randomID()
	s := &Session{ID: sid, UserID: userID, ExpiresAt: time.Now().Add(m.ttl), AccessToken: access, RefreshToken: refresh, IDToken: id, TokenExpiry: expiry}
	m.mu.Lock()
	m.mem[sid] = s
	m.mu.Unlock()
	cookie := &http.Cookie{Name: m.cookie(), Value: sid, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode}
	cookie.Expires = s.ExpiresAt
	cookie.MaxAge = int(m.ttl.Seconds())
	if isTLS(w) {
		cookie.Secure = true
	}
	http.SetCookie(w, cookie)
	m.persist(context.Background(), s, provider)
	return s
}

// Tokens returns a snapshot of tokens for the current session, if any.
func (m *Manager) Tokens(r *http.Request) (access, refresh, id string, exp time.Time) {
	s := m.Get(r)
	if s == nil {
		return "", "", "", time.Time{}
	}
	return s.AccessToken, s.RefreshToken, s.IDToken, s.TokenExpiry
}

// Get returns a live session from cookie.
func (m *Manager) Get(r *http.Request) *Session {
	c, err := r.Cookie(m.cookie())
	if err != nil || c == nil || c.Value == "" {
		return nil
	}
	sid := c.Value
	m.mu.RLock()
	s := m.mem[sid]
	m.mu.RUnlock()
	if s != nil {
		if time.Now().After(s.ExpiresAt) {
			m.removeSession(context.Background(), sid)
			return nil
		}
		return s
	}
	if m.store == nil {
		return nil
	}
	rec, err := m.store.Get(r.Context(), sid)
	if err != nil || rec == nil {
		return nil
	}
	if time.Now().After(rec.ExpiresAt) {
		_ = m.store.Delete(r.Context(), sid)
		return nil
	}
	s = &Session{ID: rec.ID, UserID: rec.UserID, ExpiresAt: rec.ExpiresAt}
	m.mu.Lock()
	m.mem[sid] = s
	m.mu.Unlock()
	return s
}

// Destroy removes session and expires the cookie.
func (m *Manager) Destroy(w http.ResponseWriter, r *http.Request) {
	c, _ := r.Cookie(m.cookie())
	if c != nil && c.Value != "" {
		m.removeSession(r.Context(), c.Value)
	}
	cookie := &http.Cookie{Name: m.cookie(), Value: "", Path: "/", HttpOnly: true, MaxAge: -1, SameSite: http.SameSiteLaxMode}
	if isTLS(w) {
		cookie.Secure = true
	}
	http.SetCookie(w, cookie)
}

func isTLS(w http.ResponseWriter) bool { return false }

func (m *Manager) removeSession(ctx context.Context, id string) {
	m.mu.Lock()
	delete(m.mem, id)
	m.mu.Unlock()
	if m.store != nil {
		_ = m.store.Delete(ctx, id)
	}
}

func (m *Manager) persist(ctx context.Context, s *Session, provider string) {
	if m == nil || m.store == nil || s == nil {
		return
	}
	provider = strings.TrimSpace(provider)
	if provider == "" {
		provider = m.defaultProvider()
	}
	rec := &SessionRecord{
		ID:        s.ID,
		UserID:    s.UserID,
		Provider:  provider,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: s.ExpiresAt,
	}
	_ = m.store.Upsert(ctx, rec)
}

func (m *Manager) defaultProvider() string {
	if m == nil || m.cfg == nil {
		return "local"
	}
	if m.cfg.OAuth != nil {
		if v := strings.TrimSpace(m.cfg.OAuth.Name); v != "" {
			return v
		}
		if strings.TrimSpace(m.cfg.OAuth.Mode) != "" {
			return "oauth"
		}
	}
	if m.cfg.Local != nil && m.cfg.Local.Enabled {
		return "local"
	}
	return "oauth"
}
