package auth

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func mkJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	header := map[string]any{"alg": "none", "typ": "JWT"}
	hb, _ := json.Marshal(header)
	pb, _ := json.Marshal(claims)
	enc := func(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
	return enc(hb) + "." + enc(pb) + ".sig"
}

func TestDecodeUserInfo(t *testing.T) {
	testCases := []struct {
		name   string
		claims map[string]any
		want   *UserInfo
	}{
		{
			name:   "email claim",
			claims: map[string]any{"email": "user@example.com"},
			want:   &UserInfo{Email: "user@example.com"},
		},
		{
			name:   "sub claim",
			claims: map[string]any{"sub": "abc-123"},
			want:   &UserInfo{Subject: "abc-123"},
		},
		{
			name:   "both claims prefer subject",
			claims: map[string]any{"sub": "u-1", "email": "e@x"},
			want:   &UserInfo{Subject: "u-1", Email: "e@x"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			token := mkJWT(t, tc.claims)
			got, err := DecodeUserInfo(token)
			assert.NoError(t, err)
			assert.EqualValues(t, tc.want, got)
		})
	}
}
