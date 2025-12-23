package chatgptauth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type jwtAuthClaims struct {
	ChatGPTAccountID string
}

func parseJWTAuthClaims(idToken string) (jwtAuthClaims, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) < 2 {
		return jwtAuthClaims{}, fmt.Errorf("invalid jwt format")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return jwtAuthClaims{}, fmt.Errorf("failed to decode jwt payload: %w", err)
	}
	var decoded any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return jwtAuthClaims{}, fmt.Errorf("failed to decode jwt payload json: %w", err)
	}
	root, ok := decoded.(map[string]any)
	if !ok {
		return jwtAuthClaims{}, fmt.Errorf("unexpected jwt payload type")
	}
	authObj, _ := root["https://api.openai.com/auth"].(map[string]any)
	chatgptAccountID, _ := authObj["chatgpt_account_id"].(string)
	return jwtAuthClaims{ChatGPTAccountID: chatgptAccountID}, nil
}

func parseJWTExpiry(token string) (time.Time, bool) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return time.Time{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, false
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return time.Time{}, false
	}
	expAny, ok := decoded["exp"]
	if !ok {
		return time.Time{}, false
	}
	switch v := expAny.(type) {
	case float64:
		return time.Unix(int64(v), 0), true
	case int64:
		return time.Unix(v, 0), true
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return time.Unix(n, 0), true
		}
	}
	return time.Time{}, false
}

func ensureWorkspaceAllowed(allowedWorkspaceID string, idToken string) error {
	if strings.TrimSpace(allowedWorkspaceID) == "" {
		return nil
	}
	claims, err := parseJWTAuthClaims(idToken)
	if err != nil {
		return err
	}
	if strings.TrimSpace(claims.ChatGPTAccountID) == "" {
		return fmt.Errorf("login is restricted to workspace %s but token lacks chatgpt_account_id", allowedWorkspaceID)
	}
	if claims.ChatGPTAccountID != allowedWorkspaceID {
		return fmt.Errorf("login is restricted to workspace %s", allowedWorkspaceID)
	}
	return nil
}
