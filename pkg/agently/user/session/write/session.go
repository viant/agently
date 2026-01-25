package write

import "time"

type Session struct {
	Id        string     `sqlx:"id,primaryKey" json:"id,omitempty"`
	UserID    string     `sqlx:"user_id" json:"userId,omitempty"`
	Provider  string     `sqlx:"provider" json:"provider,omitempty"`
	CreatedAt time.Time  `sqlx:"created_at" json:"createdAt,omitempty" format:"2006-01-02 15:04:05"`
	UpdatedAt *time.Time `sqlx:"updated_at" json:"updatedAt,omitempty" format:"2006-01-02 15:04:05"`
	ExpiresAt time.Time  `sqlx:"expires_at" json:"expiresAt,omitempty" format:"2006-01-02 15:04:05"`

	Has *SessionHas `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

type SessionHas struct {
	Id        bool
	UserID    bool
	Provider  bool
	CreatedAt bool
	UpdatedAt bool
	ExpiresAt bool
}

func (s *Session) SetID(v string) {
	s.Id = v
	if s.Has == nil {
		s.Has = &SessionHas{}
	}
	s.Has.Id = true
}

func (s *Session) SetUserID(v string) {
	s.UserID = v
	if s.Has == nil {
		s.Has = &SessionHas{}
	}
	s.Has.UserID = true
}

func (s *Session) SetProvider(v string) {
	s.Provider = v
	if s.Has == nil {
		s.Has = &SessionHas{}
	}
	s.Has.Provider = true
}

func (s *Session) SetCreatedAt(v time.Time) {
	s.CreatedAt = v
	if s.Has == nil {
		s.Has = &SessionHas{}
	}
	s.Has.CreatedAt = true
}

func (s *Session) SetUpdatedAt(v time.Time) {
	s.UpdatedAt = &v
	if s.Has == nil {
		s.Has = &SessionHas{}
	}
	s.Has.UpdatedAt = true
}

func (s *Session) SetExpiresAt(v time.Time) {
	s.ExpiresAt = v
	if s.Has == nil {
		s.Has = &SessionHas{}
	}
	s.Has.ExpiresAt = true
}
