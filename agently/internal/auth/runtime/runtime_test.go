package runtime

import (
    "context"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/viant/agently/internal/auth/authority"
    "github.com/viant/agently/internal/auth/cookiejar"
    "github.com/viant/agently/internal/auth/resolver"
    "github.com/viant/agently/internal/auth/tokens"
    scyauth "github.com/viant/scy/auth"
)

// fakeBroker implements resolver.Broker for tests
type fakeBroker struct{}

func (f *fakeBroker) Exchange(_ context.Context, _ tokens.Key, _ time.Duration) (scyauth.Token, string, error) {
    return scyauth.Token{AccessToken: "A", Expiry: time.Now().Add(time.Hour)}, "ID", nil
}
func (f *fakeBroker) Refresh(_ context.Context, _ tokens.Key, _ time.Duration) (scyauth.Token, string, error) {
    return scyauth.Token{AccessToken: "A2", Expiry: time.Now().Add(2 * time.Hour)}, "ID2", nil
}

func TestRuntime_NewResolver_AndCookieJar(t *testing.T) {
    // token store
    rs := tokens.NewMemoryRefreshStore()
    st, err := tokens.NewStore(rs, tokens.StoragePolicy{AccessInMemoryOnly: true, IDInMemoryOnly: true, RefreshEncrypted: false})
    assert.Nil(t, err)
    // cookie jar manager
    cj := cookiejar.NewManager()
    // runtime
    rt := New(cj, st, &fakeBroker{}, WithMinTTL(resolver.MinTTL{Access: 30 * time.Minute}))
    r, err := rt.NewResolver()
    assert.Nil(t, err)
    assert.NotNil(t, r)

    // cookie jar helpers
    a := authority.AuthAuthority{Issuer: "https://IDP.Example.com/realms/acme"}
    jar, key, err := rt.CookieJarForAuthority(a)
    assert.Nil(t, err)
    assert.NotNil(t, jar)
    assert.EqualValues(t, "https://idp.example.com", key)
    assert.EqualValues(t, key, rt.OriginKey(a))
}

