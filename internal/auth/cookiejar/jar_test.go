package cookiejar

import (
    "testing"

    "github.com/viant/agently/internal/auth/authority"
)

func assertEqualValues[T comparable](t *testing.T, expected, actual T, msg string) {
    if expected != actual {
        t.Fatalf("%s: expected %v, got %v", msg, expected, actual)
    }
}

func TestManager_Get_SameAuthoritySharesJar(t *testing.T) {
    m := NewManager()
    a1 := authority.AuthAuthority{Issuer: "https://idp.example.com/realms/acme"}
    a2 := authority.AuthAuthority{Origin: "https://idp.example.com"}

    jar1, k1, err := m.Get(a1)
    if err != nil { t.Fatalf("unexpected error: %v", err) }
    jar2, k2, err := m.Get(a2)
    if err != nil { t.Fatalf("unexpected error: %v", err) }

    if jar1 != jar2 {
        t.Fatalf("expected same jar instance for same authority")
    }
    assertEqualValues(t, k1, k2, "origin keys match")
}

func TestManager_Get_DifferentAuthorityDifferentJar(t *testing.T) {
    m := NewManager()
    a1 := authority.AuthAuthority{Origin: "https://idp.example.com"}
    a2 := authority.AuthAuthority{Origin: "https://idp.example.com:444"}

    jar1, k1, err := m.Get(a1)
    if err != nil { t.Fatalf("unexpected error: %v", err) }
    jar2, k2, err := m.Get(a2)
    if err != nil { t.Fatalf("unexpected error: %v", err) }

    if jar1 == jar2 {
        t.Fatalf("expected different jar instance for different authorities")
    }
    if k1 == k2 {
        t.Fatalf("expected different origin keys for different authorities")
    }
}

func TestManager_Clear(t *testing.T) {
    m := NewManager()
    a := authority.AuthAuthority{Origin: "https://idp.example.com"}
    jar1, k, err := m.Get(a)
    if err != nil { t.Fatalf("unexpected error: %v", err) }
    if k == "" { t.Fatalf("expected non-empty key") }
    m.Clear(k)
    jar2, k2, err := m.Get(a)
    if err != nil { t.Fatalf("unexpected error: %v", err) }
    if jar1 == jar2 {
        t.Fatalf("expected a new jar after clear")
    }
    assertEqualValues(t, k, k2, "origin key should be stable")
}

func TestIsRequestOriginAllowed(t *testing.T) {
    a := authority.AuthAuthority{Origin: "https://idp.example.com"}
    k := OriginKeyFromAuthority(a)
    cases := []struct{
        name string
        url  string
        want bool
    }{
        {"same origin", "https://idp.example.com/path", true},
        {"different scheme", "http://idp.example.com/path", false},
        {"different host", "https://x.idp.example.com/", false},
        {"different port", "https://idp.example.com:444/", false},
    }
    for _, tc := range cases {
        got := IsRequestOriginAllowed(tc.url, k)
        assertEqualValues(t, tc.want, got, tc.name)
    }
}

