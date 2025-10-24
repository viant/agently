package cookiejar

import (
    "net/http/cookiejar"
    "net/url"
    "sync"

    "github.com/viant/agently/internal/auth/authority"
)

// Manager provides shared cookie jars keyed by authorization authority.
// Each authority gets its own net/http CookieJar to avoid cross-authority leakage.
type Manager struct {
    mu   sync.Mutex
    jars map[string]*cookiejar.Jar // key is normalized origin (scheme://host[:port])
}

// NewManager creates a Manager with an empty set of jars.
func NewManager() *Manager {
    return &Manager{jars: make(map[string]*cookiejar.Jar)}
}

// OriginKeyFromAuthority derives a normalized origin key from the authority.
func OriginKeyFromAuthority(a authority.AuthAuthority) string {
    n := a.Normalize()
    return n.Origin
}

// Get returns a shared CookieJar for the provided authority.
// It lazily creates a new jar per unique origin key.
func (m *Manager) Get(a authority.AuthAuthority) (*cookiejar.Jar, string, error) {
    key := OriginKeyFromAuthority(a)
    if key == "" {
        // Create a dedicated jar with empty key (no reuse)
        jar, err := cookiejar.New(nil)
        return jar, "", err
    }
    m.mu.Lock()
    defer m.mu.Unlock()
    if jar, ok := m.jars[key]; ok {
        return jar, key, nil
    }
    jar, err := cookiejar.New(nil)
    if err != nil {
        return nil, key, err
    }
    m.jars[key] = jar
    return jar, key, nil
}

// Clear removes and zeroes a jar for the given authority origin key.
// Useful on logout to discard cookies.
func (m *Manager) Clear(key string) {
    if key == "" {
        return
    }
    m.mu.Lock()
    defer m.mu.Unlock()
    delete(m.jars, key)
}

// IsRequestOriginAllowed checks if the request URL belongs to the same origin key.
// The caller can use this to ensure a client with a shared jar is used for the right origin.
func IsRequestOriginAllowed(reqURL string, originKey string) bool {
    if originKey == "" {
        return false
    }
    u, err := url.Parse(reqURL)
    if err != nil || u.Scheme == "" || u.Host == "" {
        return false
    }
    target := authority.AuthAuthority{Origin: u.Scheme + "://" + u.Host}.Normalize().Origin
    return target == originKey
}

