package write

import "time"

var PackageName = "payload/write"

type Payload struct {
	Id                     string      `sqlx:"id,primaryKey" validate:"required"`
	TenantID               *string     `sqlx:"tenant_id" json:",omitempty"`
	Kind                   string      `sqlx:"kind" validate:"required"`
	Subtype                *string     `sqlx:"subtype" json:",omitempty"`
	MimeType               string      `sqlx:"mime_type" validate:"required"`
	SizeBytes              int         `sqlx:"size_bytes" validate:"required"`
	Digest                 *string     `sqlx:"digest" json:",omitempty"`
	Storage                string      `sqlx:"storage" validate:"required"`
	InlineBody             *[]byte     `sqlx:"inline_body" json:",omitempty"`
	URI                    *string     `sqlx:"uri" json:",omitempty"`
	Compression            string      `sqlx:"compression"`
	EncryptionKMSKeyID     *string     `sqlx:"encryption_kms_key_id" json:",omitempty"`
	RedactionPolicyVersion *string     `sqlx:"redaction_policy_version" json:",omitempty"`
	Redacted               *int        `sqlx:"redacted" json:",omitempty"`
	CreatedAt              *time.Time  `sqlx:"created_at" json:",omitempty"`
	SchemaRef              *string     `sqlx:"schema_ref" json:",omitempty"`
	Preview                *string     `sqlx:"preview" json:",omitempty"`
	Tags                   *string     `sqlx:"tags" json:",omitempty"`
	Has                    *PayloadHas `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

type PayloadHas struct {
	Id                     bool
	TenantID               bool
	Kind                   bool
	Subtype                bool
	MimeType               bool
	SizeBytes              bool
	Digest                 bool
	Storage                bool
	InlineBody             bool
	URI                    bool
	Compression            bool
	EncryptionKMSKeyID     bool
	RedactionPolicyVersion bool
	Redacted               bool
	CreatedAt              bool
	SchemaRef              bool
	Preview                bool
	Tags                   bool
}

func (p *Payload) ensureHas() {
	if p.Has == nil {
		p.Has = &PayloadHas{}
	}
}
func (p *Payload) SetId(v string)         { p.Id = v; p.ensureHas(); p.Has.Id = true }
func (p *Payload) SetKind(v string)       { p.Kind = v; p.ensureHas(); p.Has.Kind = true }
func (p *Payload) SetMimeType(v string)   { p.MimeType = v; p.ensureHas(); p.Has.MimeType = true }
func (p *Payload) SetSizeBytes(v int)     { p.SizeBytes = v; p.ensureHas(); p.Has.SizeBytes = true }
func (p *Payload) SetStorage(v string)    { p.Storage = v; p.ensureHas(); p.Has.Storage = true }
func (p *Payload) SetInlineBody(v []byte) { p.InlineBody = &v; p.ensureHas(); p.Has.InlineBody = true }
func (p *Payload) SetURI(v string)        { p.URI = &v; p.ensureHas(); p.Has.URI = true }
func (p *Payload) SetCompression(v string) {
	p.Compression = v
	p.ensureHas()
	p.Has.Compression = true
}
