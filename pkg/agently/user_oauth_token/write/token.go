package write

import "time"

// Token is a mutable model for PATCH upserts (internal use only).
type Token struct {
	UserID    string     `sqlx:"user_id" validate:"required"`
	Provider  string     `sqlx:"provider" validate:"required"`
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
