package authority

import (
    "testing"
)

// simple assert helpers to avoid external deps
func assertEqualValues[T comparable](t *testing.T, expected, actual T, msg string) {
    if expected != actual {
        t.Fatalf("%s: expected %v, got %v", msg, expected, actual)
    }
}

func TestNormalizeIssuer(t *testing.T) {
    cases := []struct {
        name    string
        input   string
        want    string
    }{
        {"basic https", "https://IDP.Example.com/realms/acme/", "https://idp.example.com/realms/acme"},
        {"http default port elided", "http://idp.example.com:80/", "http://idp.example.com"},
        {"https default port elided", "https://idp.example.com:443/realm", "https://idp.example.com/realm"},
        {"non-default port kept", "https://idp.example.com:444/realm/", "https://idp.example.com:444/realm"},
    }
    for _, tc := range cases {
        got := normalizeIssuer(tc.input)
        assertEqualValues(t, tc.want, got, tc.name)
    }
}

func TestNormalizeOrigin(t *testing.T) {
    cases := []struct {
        name  string
        input string
        want  string
    }{
        {"https origin", "https://IDP.Example.com/login", "https://idp.example.com"},
        {"http elide 80", "http://idp.example.com:80/path", "http://idp.example.com"},
        {"https elide 443", "https://idp.example.com:443/", "https://idp.example.com"},
        {"keep custom port", "https://idp.example.com:444", "https://idp.example.com:444"},
    }
    for _, tc := range cases {
        got := normalizeOrigin(tc.input)
        assertEqualValues(t, tc.want, got, tc.name)
    }
}

func TestSameAuthAuthority(t *testing.T) {
    cases := []struct {
        name string
        a    AuthAuthority
        b    AuthAuthority
        want bool
    }{
        {"issuer match exact", AuthAuthority{Issuer: "https://idp.example.com/realms/acme"}, AuthAuthority{Issuer: "https://idp.example.com/realms/acme/"}, true},
        {"issuer path differs", AuthAuthority{Issuer: "https://idp.example.com/realms/acme"}, AuthAuthority{Issuer: "https://idp.example.com/realms/other"}, false},
        {"origin match", AuthAuthority{Origin: "https://idp.example.com"}, AuthAuthority{Origin: "https://idp.example.com/"}, true},
        {"issuer vs origin match via origin", AuthAuthority{Issuer: "https://idp.example.com/realms/acme"}, AuthAuthority{Origin: "https://idp.example.com"}, true},
        {"different ports", AuthAuthority{Origin: "https://idp.example.com:444"}, AuthAuthority{Origin: "https://idp.example.com"}, false},
        {"different scheme", AuthAuthority{Origin: "http://idp.example.com"}, AuthAuthority{Origin: "https://idp.example.com"}, false},
    }
    for _, tc := range cases {
        got := SameAuthAuthority(tc.a, tc.b)
        assertEqualValues(t, tc.want, got, tc.name)
    }
}

func TestAllowedAuthHeader(t *testing.T) {
    allow := []string{"https://idp.example.com", "https://auth.other.com:444"}
    cases := []struct {
        name   string
        origin string
        want   bool
    }{
        {"allowed exact", "https://idp.example.com", true},
        {"allowed normalized", "https://idp.example.com/realm", true},
        {"allowed port", "https://auth.other.com:444/path", true},
        {"not allowed different port", "https://auth.other.com:445", false},
        {"not allowed host", "https://x.idp.example.com", false},
    }
    for _, tc := range cases {
        got := AllowedAuthHeader(tc.origin, allow)
        assertEqualValues(t, tc.want, got, tc.name)
    }
}

