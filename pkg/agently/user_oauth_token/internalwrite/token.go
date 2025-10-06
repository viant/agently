package internalwrite

import "time"

// Token is a mutable model for PATCH upserts (internal use only).
// Represents the user_oauth_token DB row.
type Token struct {
	UserID    string     `sqlx:"user_id,primaryKey" validate:"required"`
	Provider  string     `sqlx:"provider,primaryKey" validate:"required"`
	EncToken  string     `sqlx:"enc_token" validate:"required"`
	CreatedAt *time.Time `sqlx:"created_at" json:",omitempty"`
	UpdatedAt *time.Time `sqlx:"updated_at" json:",omitempty"`
	Has       *TokenHas  `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

type TokenHas struct{ UserID, Provider, EncToken, CreatedAt, UpdatedAt bool }

func (m *Token) ensureHas() {
	if m.Has == nil {
		m.Has = &TokenHas{}
	}
}
func (m *Token) SetUserID(v string)       { m.UserID = v; m.ensureHas(); m.Has.UserID = true }
func (m *Token) SetProvider(v string)     { m.Provider = v; m.ensureHas(); m.Has.Provider = true }
func (m *Token) SetEncToken(v string)     { m.EncToken = v; m.ensureHas(); m.Has.EncToken = true }
func (m *Token) SetCreatedAt(t time.Time) { m.CreatedAt = &t; m.ensureHas(); m.Has.CreatedAt = true }
func (m *Token) SetUpdatedAt(t time.Time) { m.UpdatedAt = &t; m.ensureHas(); m.Has.UpdatedAt = true }

type Tokens []Token

// RawToken mirrors the logical token shape (rewrite from auth.OAuthToken).
// Not persisted directly; callers should serialize+encrypt it and populate EncToken.
type RawToken struct {
	AccessToken  string    `json:"access_token,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	IDToken      string    `json:"id_token,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
}
