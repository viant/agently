package tokens

import (
	"testing"
	"time"

	"github.com/viant/agently/internal/auth/authority"
	scyauth "github.com/viant/scy/auth"
	"golang.org/x/oauth2"
)

func assertEqual[T comparable](t *testing.T, exp, got T, msg string) {
	if exp != got {
		t.Fatalf("%s: expected %v, got %v", msg, exp, got)
	}
}

func TestStore_AccessID_SetGetExpiry(t *testing.T) {
	rs := NewMemoryRefreshStore()
	store, err := NewStore(rs, StoragePolicy{AccessInMemoryOnly: true, IDInMemoryOnly: true, RefreshEncrypted: true})
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	k := Key{Authority: authority.AuthAuthority{Issuer: "https://idp.example.com/realms/acme"}, Subject: "user1", Audience: "mcp:tools"}
	now := time.Now()
	v := AccessID{
		Access:       scyauth.Token{Token: oauth2.Token{AccessToken: "access123"}},
		AccessExpiry: now.Add(2 * time.Hour),
		IDToken:      "id123",
		IDExpiry:     now.Add(1 * time.Hour),
		Issuer:       "https://idp.example.com/realms/acme",
		Scopes:       []string{"openid", "profile"},
	}
	store.SetAccessID(k, v)

	got, ok := store.GetAccessID(k)
	if !ok {
		t.Fatalf("expected tokens present")
	}
	assertEqual(t, "access123", got.Access.AccessToken, "access token")
	assertEqual(t, "id123", got.IDToken, "id token")

	// Expire ID token
	v.IDExpiry = now.Add(-1 * time.Minute)
	store.SetAccessID(k, v)
	got, ok = store.GetAccessID(k)
	if !ok {
		t.Fatalf("expected access present even if id expired")
	}
	assertEqual(t, "access123", got.Access.AccessToken, "access token still present")
	assertEqual(t, true, got.IDToken == "", "id token cleared when expired")
}

func TestStore_Refresh_CRUD(t *testing.T) {
	rs := NewMemoryRefreshStore()
	store, err := NewStore(rs, StoragePolicy{RefreshEncrypted: true})
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	k := Key{Authority: authority.AuthAuthority{Origin: "https://idp.example.com"}, Subject: "user1", Audience: "mcp:tools"}
	r := Refresh{Token: FromString("refresh123"), Expiry: time.Now().Add(24 * time.Hour)}
	if err := store.SetRefresh(k, r); err != nil {
		t.Fatalf("set refresh: %v", err)
	}
	got, ok, err := store.GetRefresh(k)
	if err != nil {
		t.Fatalf("get refresh: %v", err)
	}
	if !ok {
		t.Fatalf("expected refresh present")
	}
	assertEqual(t, "refresh123", got.Token.String(), "refresh value")
	if err := store.DeleteRefresh(k); err != nil {
		t.Fatalf("delete refresh: %v", err)
	}
	_, ok, err = store.GetRefresh(k)
	if err != nil {
		t.Fatalf("get refresh after delete: %v", err)
	}
	assertEqual(t, false, ok, "refresh should be gone")
}

func TestStore_DeleteAccessID_Zeroize(t *testing.T) {
	store, err := NewStore(NewMemoryRefreshStore(), StoragePolicy{RefreshEncrypted: true})
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	k := Key{Authority: authority.AuthAuthority{Origin: "https://idp.example.com"}, Subject: "user1", Audience: "mcp:tools"}
	v := AccessID{Access: scyauth.Token{Token: oauth2.Token{AccessToken: "acc"}}, AccessExpiry: time.Now().Add(1 * time.Hour), IDToken: "id", IDExpiry: time.Now().Add(1 * time.Hour)}
	store.SetAccessID(k, v)
	store.DeleteAccessID(k)
	_, ok := store.GetAccessID(k)
	assertEqual(t, false, ok, "access/id removed")
}

func TestStore_Logout(t *testing.T) {
	rs := NewMemoryRefreshStore()
	store, err := NewStore(rs, StoragePolicy{RefreshEncrypted: true})
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	k := Key{Authority: authority.AuthAuthority{Origin: "https://idp.example.com"}, Subject: "user1", Audience: "mcp:tools"}
	v := AccessID{Access: scyauth.Token{Token: oauth2.Token{AccessToken: "acc"}}, AccessExpiry: time.Now().Add(1 * time.Hour), IDToken: "id", IDExpiry: time.Now().Add(1 * time.Hour)}
	store.SetAccessID(k, v)
	_ = store.SetRefresh(k, Refresh{Token: FromString("refresh"), Expiry: time.Now().Add(24 * time.Hour)})
	if err := store.Logout(k); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, ok := store.GetAccessID(k); ok {
		t.Fatalf("expected no access/id after logout")
	}
	if _, ok, _ := store.GetRefresh(k); ok {
		t.Fatalf("expected no refresh after logout")
	}
}
