package auth

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
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

type Manager struct {
	cfg   *Config
	mu    sync.RWMutex
	store map[string]*Session
	ttl   time.Duration
}

func NewManager(cfg *Config) *Manager {
	ttl := 7 * 24 * time.Hour
	return &Manager{cfg: cfg, store: map[string]*Session{}, ttl: ttl}
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
	sid := m.randomID()
	s := &Session{ID: sid, UserID: userID, ExpiresAt: time.Now().Add(m.ttl)}
	m.mu.Lock()
	m.store[sid] = s
	m.mu.Unlock()
	cookie := &http.Cookie{Name: m.cookie(), Value: sid, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode}
	if isTLS(w) {
		cookie.Secure = true
	}
	http.SetCookie(w, cookie)
	return s
}

// CreateWithTokens stores a session with OAuth tokens (BFF) and sets the cookie.
func (m *Manager) CreateWithTokens(w http.ResponseWriter, userID, access, refresh, id string, expiry time.Time) *Session {
	sid := m.randomID()
	s := &Session{ID: sid, UserID: userID, ExpiresAt: time.Now().Add(m.ttl), AccessToken: access, RefreshToken: refresh, IDToken: id, TokenExpiry: expiry}
	m.mu.Lock()
	m.store[sid] = s
	m.mu.Unlock()
	cookie := &http.Cookie{Name: m.cookie(), Value: sid, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode}
	if isTLS(w) {
		cookie.Secure = true
	}
	http.SetCookie(w, cookie)
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
	s := m.store[sid]
	m.mu.RUnlock()
	if s == nil || time.Now().After(s.ExpiresAt) {
		return nil
	}
	return s
}

// Destroy removes session and expires the cookie.
func (m *Manager) Destroy(w http.ResponseWriter, r *http.Request) {
	c, _ := r.Cookie(m.cookie())
	if c != nil && c.Value != "" {
		m.mu.Lock()
		delete(m.store, c.Value)
		m.mu.Unlock()
	}
	cookie := &http.Cookie{Name: m.cookie(), Value: "", Path: "/", HttpOnly: true, MaxAge: -1, SameSite: http.SameSiteLaxMode}
	if isTLS(w) {
		cookie.Secure = true
	}
	http.SetCookie(w, cookie)
}

func isTLS(w http.ResponseWriter) bool { return false }
