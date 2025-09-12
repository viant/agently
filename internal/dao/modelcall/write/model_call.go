package write

import "time"

var PackageName = "modelcall/write"

type ModelCall struct {
	MessageID         string        `sqlx:"message_id,primaryKey" validate:"required"`
	TurnID            *string       `sqlx:"turn_id" json:",omitempty"`
	Provider          string        `sqlx:"provider" validate:"required"`
	Model             string        `sqlx:"model" validate:"required"`
	ModelKind         string        `sqlx:"model_kind" validate:"required"`
	PromptTokens      *int          `sqlx:"prompt_tokens" json:",omitempty"`
	CompletionTokens  *int          `sqlx:"completion_tokens" json:",omitempty"`
	TotalTokens       *int          `sqlx:"total_tokens" json:",omitempty"`
	FinishReason      *string       `sqlx:"finish_reason" json:",omitempty"`
	CacheHit          *int          `sqlx:"cache_hit" json:",omitempty"`
	CacheKey          *string       `sqlx:"cache_key" json:",omitempty"`
	StartedAt         *time.Time    `sqlx:"started_at" json:",omitempty"`
	CompletedAt       *time.Time    `sqlx:"completed_at" json:",omitempty"`
	LatencyMS         *int          `sqlx:"latency_ms" json:",omitempty"`
	Cost              *float64      `sqlx:"cost" json:",omitempty"`
	TraceID           *string       `sqlx:"trace_id" json:",omitempty"`
	SpanID            *string       `sqlx:"span_id" json:",omitempty"`
	RequestPayloadID  *string       `sqlx:"request_payload_id" json:",omitempty"`
	ResponsePayloadID *string       `sqlx:"response_payload_id" json:",omitempty"`
	StreamPayloadID   *string       `sqlx:"stream_payload_id" json:",omitempty"`
	Has               *ModelCallHas `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

type ModelCallHas struct {
	MessageID         bool
	TurnID            bool
	Provider          bool
	Model             bool
	ModelKind         bool
	PromptTokens      bool
	CompletionTokens  bool
	TotalTokens       bool
	FinishReason      bool
	CacheHit          bool
	CacheKey          bool
	StartedAt         bool
	CompletedAt       bool
	LatencyMS         bool
	Cost              bool
	TraceID           bool
	SpanID            bool
	RequestPayloadID  bool
	ResponsePayloadID bool
	StreamPayloadID   bool
}

func (m *ModelCall) ensureHas() {
	if m.Has == nil {
		m.Has = &ModelCallHas{}
	}
}
func (m *ModelCall) SetMessageID(v string) { m.MessageID = v; m.ensureHas(); m.Has.MessageID = true }
func (m *ModelCall) SetProvider(v string)  { m.Provider = v; m.ensureHas(); m.Has.Provider = true }
func (m *ModelCall) SetModel(v string)     { m.Model = v; m.ensureHas(); m.Has.Model = true }
func (m *ModelCall) SetModelKind(v string) { m.ModelKind = v; m.ensureHas(); m.Has.ModelKind = true }
func (m *ModelCall) SetStreamPayloadID(v string) {
	m.StreamPayloadID = &v
	m.ensureHas()
	m.Has.StreamPayloadID = true
}
