package tokens

import base "github.com/viant/agently/internal/auth/tokens"

// KeyProvider supplies raw key material for encrypting refresh tokens.
type KeyProvider interface {
	Key() ([]byte, error)
}

// Type aliases to share implementations with the core internal/auth/tokens package.
type (
	Key           = base.Key
	AccessID      = base.AccessID
	Refresh       = base.Refresh
	RefreshStore  = base.RefreshStore
	StoragePolicy = base.StoragePolicy
	Store         = base.Store
	Secret        = base.Secret
)

// NewStore forwards construction to the shared Store implementation.
func NewStore(refresh RefreshStore, policy StoragePolicy) (*Store, error) {
	return base.NewStore(refresh, policy)
}

// NewMemoryRefreshStore returns an in-memory refresh store for tests and ephemeral sessions.
func NewMemoryRefreshStore() *base.MemoryRefreshStore {
	return base.NewMemoryRefreshStore()
}

// FromString constructs a Secret from a string while enabling zeroization.
func FromString(s string) Secret {
	return base.FromString(s)
}
