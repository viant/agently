package write

import "time"

var PackageName = "generatedfile/write"

type GeneratedFile struct {
	ID             string            `sqlx:"id,primaryKey" validate:"required"`
	ConversationID string            `sqlx:"conversation_id" validate:"required"`
	TurnID         *string           `sqlx:"turn_id" json:",omitempty"`
	MessageID      *string           `sqlx:"message_id" json:",omitempty"`
	Provider       string            `sqlx:"provider" validate:"required"`
	Mode           string            `sqlx:"mode" validate:"required"`
	CopyMode       string            `sqlx:"copy_mode" validate:"required"`
	Status         string            `sqlx:"status" validate:"required"`
	PayloadID      *string           `sqlx:"payload_id" json:",omitempty"`
	ContainerID    *string           `sqlx:"container_id" json:",omitempty"`
	ProviderFileID *string           `sqlx:"provider_file_id" json:",omitempty"`
	Filename       *string           `sqlx:"filename" json:",omitempty"`
	MimeType       *string           `sqlx:"mime_type" json:",omitempty"`
	SizeBytes      *int              `sqlx:"size_bytes" json:",omitempty"`
	Checksum       *string           `sqlx:"checksum" json:",omitempty"`
	ErrorMessage   *string           `sqlx:"error_message" json:",omitempty"`
	ExpiresAt      *time.Time        `sqlx:"expires_at" json:",omitempty"`
	CreatedAt      *time.Time        `sqlx:"created_at" json:",omitempty"`
	UpdatedAt      *time.Time        `sqlx:"updated_at" json:",omitempty"`
	Has            *GeneratedFileHas `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

type GeneratedFileHas struct {
	ID             bool
	ConversationID bool
	TurnID         bool
	MessageID      bool
	Provider       bool
	Mode           bool
	CopyMode       bool
	Status         bool
	PayloadID      bool
	ContainerID    bool
	ProviderFileID bool
	Filename       bool
	MimeType       bool
	SizeBytes      bool
	Checksum       bool
	ErrorMessage   bool
	ExpiresAt      bool
	CreatedAt      bool
	UpdatedAt      bool
}

func (p *GeneratedFile) ensureHas() {
	if p.Has == nil {
		p.Has = &GeneratedFileHas{}
	}
}
func (p *GeneratedFile) SetID(v string) { p.ID = v; p.ensureHas(); p.Has.ID = true }
func (p *GeneratedFile) SetConversationID(v string) {
	p.ConversationID = v
	p.ensureHas()
	p.Has.ConversationID = true
}
func (p *GeneratedFile) SetTurnID(v string) { p.TurnID = &v; p.ensureHas(); p.Has.TurnID = true }
func (p *GeneratedFile) SetMessageID(v string) {
	p.MessageID = &v
	p.ensureHas()
	p.Has.MessageID = true
}
func (p *GeneratedFile) SetProvider(v string) { p.Provider = v; p.ensureHas(); p.Has.Provider = true }
func (p *GeneratedFile) SetMode(v string)     { p.Mode = v; p.ensureHas(); p.Has.Mode = true }
func (p *GeneratedFile) SetCopyMode(v string) { p.CopyMode = v; p.ensureHas(); p.Has.CopyMode = true }
func (p *GeneratedFile) SetStatus(v string)   { p.Status = v; p.ensureHas(); p.Has.Status = true }
func (p *GeneratedFile) SetPayloadID(v string) {
	p.PayloadID = &v
	p.ensureHas()
	p.Has.PayloadID = true
}
func (p *GeneratedFile) SetContainerID(v string) {
	p.ContainerID = &v
	p.ensureHas()
	p.Has.ContainerID = true
}
func (p *GeneratedFile) SetProviderFileID(v string) {
	p.ProviderFileID = &v
	p.ensureHas()
	p.Has.ProviderFileID = true
}
func (p *GeneratedFile) SetFilename(v string) { p.Filename = &v; p.ensureHas(); p.Has.Filename = true }
func (p *GeneratedFile) SetMimeType(v string) { p.MimeType = &v; p.ensureHas(); p.Has.MimeType = true }
func (p *GeneratedFile) SetSizeBytes(v int)   { p.SizeBytes = &v; p.ensureHas(); p.Has.SizeBytes = true }
func (p *GeneratedFile) SetChecksum(v string) { p.Checksum = &v; p.ensureHas(); p.Has.Checksum = true }
func (p *GeneratedFile) SetErrorMessage(v string) {
	p.ErrorMessage = &v
	p.ensureHas()
	p.Has.ErrorMessage = true
}
func (p *GeneratedFile) SetExpiresAt(v time.Time) {
	p.ExpiresAt = &v
	p.ensureHas()
	p.Has.ExpiresAt = true
}
func (p *GeneratedFile) SetCreatedAt(v time.Time) {
	p.CreatedAt = &v
	p.ensureHas()
	p.Has.CreatedAt = true
}
func (p *GeneratedFile) SetUpdatedAt(v time.Time) {
	p.UpdatedAt = &v
	p.ensureHas()
	p.Has.UpdatedAt = true
}
