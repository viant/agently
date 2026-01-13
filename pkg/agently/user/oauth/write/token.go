package write

import (
	"time"
)

type Token struct {
	UserID    string     `sqlx:"user_id,primaryKey" json:"userId,omitempty" diff:"-"`
	Provider  string     `sqlx:"provider,primaryKey" json:"provider,omitempty"`
	EncToken  string     `sqlx:"enc_token" json:"encToken,omitempty"`
	CreatedAt time.Time  `sqlx:"created_at" json:"createdAt,omitempty" format:"2006-01-02 15:04:05"`
	UpdatedAt *time.Time `sqlx:"updated_at" json:"updatedAt,omitempty" format:"2006-01-02 15:04:05"`

	Has *TokenHas `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

type TokenHas struct {
	UserID    bool
	Provider  bool
	EncToken  bool
	CreatedAt bool
	UpdatedAt bool
}

func (t *Token) SetUserID(v string) {
	t.UserID = v
	if t.Has == nil {
		t.Has = &TokenHas{}
	}
	t.Has.UserID = true
}
func (t *Token) SetProvider(v string) {
	t.Provider = v
	if t.Has == nil {
		t.Has = &TokenHas{}
	}
	t.Has.Provider = true
}
func (t *Token) SetEncToken(v string) {
	t.EncToken = v
	if t.Has == nil {
		t.Has = &TokenHas{}
	}
	t.Has.EncToken = true
}
func (t *Token) SetCreatedAt(v time.Time) {
	t.CreatedAt = v
	if t.Has == nil {
		t.Has = &TokenHas{}
	}
	t.Has.CreatedAt = true
}
func (t *Token) SetUpdatedAt(v time.Time) {
	t.UpdatedAt = &v
	if t.Has == nil {
		t.Has = &TokenHas{}
	}
	t.Has.UpdatedAt = true
}
