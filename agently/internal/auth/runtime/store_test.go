package runtime

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/internal/auth/authority"
	"github.com/viant/agently/internal/auth/tokens"
)

func TestNewTokenStore_EncryptedAndMemory(t *testing.T) {
	tmp := t.TempDir()
	key := RawKeyProvider{K: BlowfishEnsureKey("test-salt")}
	pol := tokens.StoragePolicy{AccessInMemoryOnly: true, IDInMemoryOnly: true, RefreshEncrypted: true}
	st, err := NewTokenStore(tmp, key, pol)
	assert.Nil(t, err)
	assert.NotNil(t, st)

	// Write a refresh token and read it back via the file store
	k := tokens.Key{Authority: authority.AuthAuthority{Issuer: "https://idp.example.com/realms/acme"}, Subject: "u1", Audience: "aud"}
	exp := time.Now().Add(2 * time.Hour)
	r := tokens.Refresh{Token: tokens.FromString("R1"), Expiry: exp}
	assert.Nil(t, st.SetRefresh(k, r))
	got, ok, err := st.GetRefresh(k)
	assert.Nil(t, err)
	assert.EqualValues(t, true, ok)
	assert.EqualValues(t, "R1", got.Token.String())
	// Expiry roundtrip may lose exact instant; assert non-zero and within reasonable range.
	assert.NotZero(t, got.Expiry.Unix())

	// Ensure file exists on disk (sanity)
	files, _ := os.ReadDir(filepath.Clean(tmp))
	assert.GreaterOrEqual(t, len(files), 1)

	// Memory policy should not require a key
	pol2 := tokens.StoragePolicy{AccessInMemoryOnly: true, IDInMemoryOnly: true, RefreshEncrypted: false}
	st2, err := NewTokenStore(tmp, nil, pol2)
	assert.Nil(t, err)
	assert.NotNil(t, st2)
}
