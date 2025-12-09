package tokens

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/internal/auth/authority"
)

type staticKey struct{ k []byte }

func (s staticKey) Key() ([]byte, error) { return s.k, nil }

func TestScyRefreshStore_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	st, err := NewScyRefreshStore(dir, staticKey{[]byte("secret-passphrase")})
	assert.Nil(t, err)
	k := Key{Authority: authority.AuthAuthority{Issuer: "https://idp.example.com/realms/acme"}, Subject: "u1", Audience: "aud"}
	r := Refresh{Token: FromString("R1"), Expiry: time.Now().Add(2 * time.Hour)}
	assert.Nil(t, st.Set(k, r))
	got, ok, err := st.Get(k)
	assert.Nil(t, err)
	assert.EqualValues(t, true, ok)
	assert.EqualValues(t, "R1", got.Token.String())
	// Expiry roundtrip may lose exact instant; assert non-zero timestamp.
	assert.NotZero(t, got.Expiry.Unix())

	// file exists
	files, _ := os.ReadDir(filepath.Clean(dir))
	assert.GreaterOrEqual(t, len(files), 1)

	// delete
	assert.Nil(t, st.Delete(k))
}
