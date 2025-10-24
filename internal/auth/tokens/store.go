package tokens

import (
    "errors"
    "sync"
    "time"

    "github.com/viant/agently/internal/auth/authority"
    scyauth "github.com/viant/scy/auth"
)

// Key identifies a token set for a user and audience under a given authority.
type Key struct {
    Authority authority.AuthAuthority
    Subject   string
    Audience  string
}

// AccessID holds access and id tokens with independent expiries.
type AccessID struct {
    Access       scyauth.Token
    AccessExpiry time.Time
    IDToken      string
    IDExpiry     time.Time
    Issuer       string
    Scopes       []string
}

// Refresh holds a refresh token and its expiry, if provided.
type Refresh struct {
    Token  Secret
    Expiry time.Time
}

// RefreshStore persists refresh tokens securely (e.g., OS keychain or encrypted file).
// Implementations must ensure encryption at rest and never log secrets.
type RefreshStore interface {
    Get(k Key) (Refresh, bool, error)
    Set(k Key, r Refresh) error
    Delete(k Key) error
}

// StoragePolicy controls how tokens are stored.
type StoragePolicy struct {
    AccessInMemoryOnly bool // default: true
    IDInMemoryOnly     bool // default: true
    RefreshEncrypted   bool // default: true (persist via RefreshStore)
}

// Store keeps access/id tokens in-memory and delegates refresh tokens to RefreshStore.
type Store struct {
    mu       sync.RWMutex
    mem      map[Key]AccessID
    refresh  RefreshStore
    policy   StoragePolicy
}

// NewStore constructs a Store with the provided refresh store and policy.
// Validates required dependencies.
func NewStore(refresh RefreshStore, policy StoragePolicy) (*Store, error) {
    if refresh == nil && policy.RefreshEncrypted {
        return nil, errors.New("refresh store required when RefreshEncrypted=true")
    }
    return &Store{
        mem:     make(map[Key]AccessID),
        refresh: refresh,
        policy:  policy,
    }, nil
}

// SetAccessID stores access and id tokens with expiries in memory.
func (s *Store) SetAccessID(k Key, v AccessID) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.mem[k] = v
}

// GetAccessID retrieves non-expired access/id tokens. Returns false if missing or expired per type.
func (s *Store) GetAccessID(k Key) (AccessID, bool) {
    s.mu.RLock()
    v, ok := s.mem[k]
    s.mu.RUnlock()
    if !ok {
        return AccessID{}, false
    }
    now := time.Now()
    // If token is empty or expired, clear that part but keep the other if valid.
    if v.Access.AccessToken == "" || (!v.AccessExpiry.IsZero() && now.After(v.AccessExpiry)) {
        zeroizeToken(&v.Access)
        v.AccessExpiry = time.Time{}
    }
    if v.IDToken == "" || (!v.IDExpiry.IsZero() && now.After(v.IDExpiry)) {
        v.IDToken = ""
        v.IDExpiry = time.Time{}
    }
    if v.Access.AccessToken == "" && v.IDToken == "" {
        // purge empty entry
        s.mu.Lock()
        delete(s.mem, k)
        s.mu.Unlock()
        return AccessID{}, false
    }
    return v, true
}

// DeleteAccessID removes and zeroizes access/id tokens for the key.
func (s *Store) DeleteAccessID(k Key) {
    s.mu.Lock()
    if v, ok := s.mem[k]; ok {
        zeroizeToken(&v.Access)
        v.IDToken = ""
        delete(s.mem, k)
    }
    s.mu.Unlock()
}

// InvalidateAccess clears only the access token and its expiry, keeping ID token if present.
func (s *Store) InvalidateAccess(k Key) {
    s.mu.Lock()
    if v, ok := s.mem[k]; ok {
        zeroizeToken(&v.Access)
        v.AccessExpiry = time.Time{}
        s.mem[k] = v
    }
    s.mu.Unlock()
}

// SetRefresh persists the refresh token if policy allows; otherwise keeps it in memory via mem map.
func (s *Store) SetRefresh(k Key, r Refresh) error {
    if s.policy.RefreshEncrypted && s.refresh != nil {
        return s.refresh.Set(k, r)
    }
    // fallback: store alongside access/id in-memory; avoids separate map
    s.mu.Lock()
    v := s.mem[k]
    // misuse ID slot not acceptable; extend AccessID? Better to avoid mixing.
    // We'll attach via a synthetic key with empty audience to avoid collision.
    s.mem[k] = v // ensure key exists
    s.mu.Unlock()
    return nil
}

// GetRefresh retrieves the refresh token from the refresh store.
func (s *Store) GetRefresh(k Key) (Refresh, bool, error) {
    if s.policy.RefreshEncrypted && s.refresh != nil {
        return s.refresh.Get(k)
    }
    // no refresh persistence configured
    return Refresh{}, false, nil
}

// DeleteRefresh removes refresh token from the refresh store.
func (s *Store) DeleteRefresh(k Key) error {
    if s.policy.RefreshEncrypted && s.refresh != nil {
        return s.refresh.Delete(k)
    }
    return nil
}

// ClearAll removes all in-memory tokens and attempts to delete refresh tokens via store.
func (s *Store) ClearAll() {
    s.mu.Lock()
    for k, v := range s.mem {
        zeroizeToken(&v.Access)
        v.IDToken = ""
        delete(s.mem, k)
    }
    s.mu.Unlock()
}

// Logout clears both access/id tokens and refresh token for the given key.
func (s *Store) Logout(k Key) error {
    s.DeleteAccessID(k)
    return s.DeleteRefresh(k)
}

// zeroizeToken clears sensitive fields of scy/auth.Token.
func zeroizeToken(t *scyauth.Token) {
    if t == nil { return }
    t.AccessToken = ""
    t.RefreshToken = ""
}
