package write

import "time"

var PackageName = "user/write"

// User is a mutable model for PATCH upserts. Server controls hash_ip.
type User struct {
	Id                 string     `sqlx:"id,primaryKey" validate:"required"`
	Username           string     `sqlx:"username" validate:"required"`
	DisplayName        *string    `sqlx:"display_name" json:",omitempty"`
	Email              *string    `sqlx:"email" json:",omitempty"`
	Provider           string     `sqlx:"provider" validate:"required"`
	Subject            *string    `sqlx:"subject" json:",omitempty"`
	HashIP             *string    `sqlx:"hash_ip" json:"-"`
	Timezone           string     `sqlx:"timezone" validate:"required"`
	DefaultAgentRef    *string    `sqlx:"default_agent_ref" json:",omitempty"`
	DefaultModelRef    *string    `sqlx:"default_model_ref" json:",omitempty"`
	DefaultEmbedderRef *string    `sqlx:"default_embedder_ref" json:",omitempty"`
	Settings           *string    `sqlx:"settings" json:",omitempty"`
	Disabled           *int       `sqlx:"disabled" json:",omitempty"`
	CreatedAt          *time.Time `sqlx:"created_at" json:",omitempty"`
	UpdatedAt          *time.Time `sqlx:"updated_at" json:",omitempty"`
	Has                *UserHas   `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

type UserHas struct {
	Id, Username, DisplayName, Email, Provider, Subject, HashIP    bool
	Timezone, DefaultAgentRef, DefaultModelRef, DefaultEmbedderRef bool
	Settings, Disabled, CreatedAt, UpdatedAt                       bool
}

func (m *User) ensureHas() {
	if m.Has == nil {
		m.Has = &UserHas{}
	}
}
func (m *User) SetId(v string)          { m.Id = v; m.ensureHas(); m.Has.Id = true }
func (m *User) SetUsername(v string)    { m.Username = v; m.ensureHas(); m.Has.Username = true }
func (m *User) SetDisplayName(v string) { m.DisplayName = &v; m.ensureHas(); m.Has.DisplayName = true }
func (m *User) SetEmail(v string)       { m.Email = &v; m.ensureHas(); m.Has.Email = true }
func (m *User) SetProvider(v string)    { m.Provider = v; m.ensureHas(); m.Has.Provider = true }
func (m *User) SetSubject(v string)     { m.Subject = &v; m.ensureHas(); m.Has.Subject = true }
func (m *User) SetHashIP(v string)      { m.HashIP = &v; m.ensureHas(); m.Has.HashIP = true }
func (m *User) SetTimezone(v string)    { m.Timezone = v; m.ensureHas(); m.Has.Timezone = true }
func (m *User) SetDefaultAgentRef(v string) {
	m.DefaultAgentRef = &v
	m.ensureHas()
	m.Has.DefaultAgentRef = true
}
func (m *User) SetDefaultModelRef(v string) {
	m.DefaultModelRef = &v
	m.ensureHas()
	m.Has.DefaultModelRef = true
}
func (m *User) SetDefaultEmbedderRef(v string) {
	m.DefaultEmbedderRef = &v
	m.ensureHas()
	m.Has.DefaultEmbedderRef = true
}
func (m *User) SetSettings(v string)     { m.Settings = &v; m.ensureHas(); m.Has.Settings = true }
func (m *User) SetDisabled(v int)        { m.Disabled = &v; m.ensureHas(); m.Has.Disabled = true }
func (m *User) SetCreatedAt(v time.Time) { m.CreatedAt = &v; m.ensureHas(); m.Has.CreatedAt = true }
func (m *User) SetUpdatedAt(v time.Time) { m.UpdatedAt = &v; m.ensureHas(); m.Has.UpdatedAt = true }

// Users slice for dataType reference in structql markers.
type Users []User
