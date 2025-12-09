package runtime

import (
	"errors"

	"github.com/viant/agently/agently/internal/auth/tokens"
	"github.com/viant/scy/kms/blowfish"
)

// RawKeyProvider implements tokens.KeyProvider using provided key bytes.
// Use BlowfishEnsureKey to derive a 32-byte key from a passphrase when needed.
type RawKeyProvider struct{ K []byte }

func (r RawKeyProvider) Key() ([]byte, error) {
	if len(r.K) == 0 {
		return nil, errors.New("key provider: empty key")
	}
	return r.K, nil
}

// BlowfishEnsureKey derives a stable 32-byte key from the given salt/passphrase.
// It uses the same derivation as other blowfish helpers for consistency.
func BlowfishEnsureKey(salt string) []byte { return blowfish.EnsureKey([]byte(salt)) }

// NewTokenStore constructs a tokens.Store with an encrypted file-backed refresh store.
// - refreshDir: directory for encrypted refresh blobs (created with 0700 perms)
// - key: key provider for encrypting refresh tokens
// - policy: storage policy for access/id and refresh tokens
func NewTokenStore(refreshDir string, key tokens.KeyProvider, policy tokens.StoragePolicy) (*tokens.Store, error) {
	if policy.RefreshEncrypted && key == nil {
		return nil, errors.New("token store: key provider required when RefreshEncrypted=true")
	}
	var rs tokens.RefreshStore
	var err error
	if policy.RefreshEncrypted {
		rs, err = tokens.NewScyRefreshStore(refreshDir, key)
		if err != nil {
			return nil, err
		}
	} else {
		rs = tokens.NewMemoryRefreshStore()
	}
	return tokens.NewStore(rs, policy)
}
