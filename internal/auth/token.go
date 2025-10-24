package auth

import (
	"time"
)

// OAuthToken is a minimal serialized token shape stored encrypted in DB.
type OAuthToken struct {
	AccessToken  string    `json:"access_token,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	IDToken      string    `json:"id_token,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
}
