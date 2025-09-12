package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// DecodeUserInfo parses a JWT token payload without verifying signature and
// extracts common identity fields (email, sub). It returns nil when parsing
// fails or when no useful claims are present.
func DecodeUserInfo(token string) (*UserInfo, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, nil
	}
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid token format")
	}
	payloadSeg := parts[1]
	// JWTs use base64url (no padding). Add padding if necessary.
	if m := len(payloadSeg) % 4; m != 0 {
		payloadSeg += strings.Repeat("=", 4-m)
	}
	data, err := base64.URLEncoding.DecodeString(payloadSeg)
	if err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}
	var claims map[string]interface{}
	if err := json.Unmarshal(data, &claims); err != nil {
		return nil, fmt.Errorf("decode claims: %w", err)
	}
	out := &UserInfo{}
	if v, ok := claims["email"].(string); ok && strings.TrimSpace(v) != "" {
		out.Email = v
	}
	if v, ok := claims["sub"].(string); ok && strings.TrimSpace(v) != "" {
		out.Subject = v
	}
	if out.Email == "" && out.Subject == "" {
		return nil, nil
	}
	return out, nil
}

// ExtractBearer strips the Bearer prefix from an Authorization header value.
func ExtractBearer(authzHeader string) string {
	h := strings.TrimSpace(authzHeader)
	if h == "" {
		return ""
	}
	if len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return h
}
