package server

import (
	"strings"
	"testing"
)

func TestRequireSecureTransport(t *testing.T) {
	cases := []struct {
		name    string
		base    string
		wantErr string // empty means no error expected
	}{
		{"https public", "https://api.openai.com", ""},
		{"https with port", "https://proxy.example.com:8443", ""},
		{"http localhost allowed", "http://localhost:8080", ""},
		{"http loopback v4 allowed", "http://127.0.0.1:1234", ""},
		{"http loopback v6 allowed", "http://[::1]:1234", ""},
		{"http 127-net allowed", "http://127.5.6.7", ""},
		{"http public rejected", "http://api.openai.com", "plaintext"},
		{"http lan rejected", "http://192.168.1.5:8080", "plaintext"},
		{"unsupported scheme", "ftp://example.com", "unsupported"},
		{"malformed", "://bad", "invalid"},
	}
	for _, tc := range cases {
		err := requireSecureTransport(tc.base)
		if tc.wantErr == "" {
			if err != nil {
				t.Errorf("%s: unexpected error %v", tc.name, err)
			}
			continue
		}
		if err == nil {
			t.Errorf("%s: expected error containing %q, got nil", tc.name, tc.wantErr)
			continue
		}
		if !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("%s: expected error containing %q, got %v", tc.name, tc.wantErr, err)
		}
	}
}
