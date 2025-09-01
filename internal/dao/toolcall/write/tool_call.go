package write

import "time"

var PackageName = "toolcall/write"

type ToolCall struct {
	MessageID        string       `sqlx:"message_id,primaryKey" validate:"required"`
	TurnID           *string      `sqlx:"turn_id" json:",omitempty"`
	OpID             string       `sqlx:"op_id" validate:"required"`
	Attempt          int          `sqlx:"attempt"`
	ToolName         string       `sqlx:"tool_name" validate:"required"`
	ToolKind         string       `sqlx:"tool_kind" validate:"required"`
	CapabilityTags   *string      `sqlx:"capability_tags" json:",omitempty"`
	ResourceURIs     *string      `sqlx:"resource_uris" json:",omitempty"`
	Status           string       `sqlx:"status" validate:"required"`
	RequestSnapshot  *string      `sqlx:"request_snapshot" json:",omitempty"`
	RequestHash      *string      `sqlx:"request_hash" json:",omitempty"`
	ResponseSnapshot *string      `sqlx:"response_snapshot" json:",omitempty"`
	ErrorCode        *string      `sqlx:"error_code" json:",omitempty"`
	ErrorMessage     *string      `sqlx:"error_message" json:",omitempty"`
	Retriable        *int         `sqlx:"retriable" json:",omitempty"`
	StartedAt        *time.Time   `sqlx:"started_at" json:",omitempty"`
	CompletedAt      *time.Time   `sqlx:"completed_at" json:",omitempty"`
	LatencyMS        *int         `sqlx:"latency_ms" json:",omitempty"`
	Cost             *float64     `sqlx:"cost" json:",omitempty"`
	TraceID          *string      `sqlx:"trace_id" json:",omitempty"`
	SpanID           *string      `sqlx:"span_id" json:",omitempty"`
	Has              *ToolCallHas `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

type ToolCallHas struct {
	MessageID        bool
	TurnID           bool
	OpID             bool
	Attempt          bool
	ToolName         bool
	ToolKind         bool
	CapabilityTags   bool
	ResourceURIs     bool
	Status           bool
	RequestSnapshot  bool
	RequestHash      bool
	ResponseSnapshot bool
	ErrorCode        bool
	ErrorMessage     bool
	Retriable        bool
	StartedAt        bool
	CompletedAt      bool
	LatencyMS        bool
	Cost             bool
	TraceID          bool
	SpanID           bool
}

func (t *ToolCall) ensureHas() {
	if t.Has == nil {
		t.Has = &ToolCallHas{}
	}
}
func (t *ToolCall) SetMessageID(v string) { t.MessageID = v; t.ensureHas(); t.Has.MessageID = true }
func (t *ToolCall) SetOpID(v string)      { t.OpID = v; t.ensureHas(); t.Has.OpID = true }
func (t *ToolCall) SetAttempt(v int)      { t.Attempt = v; t.ensureHas(); t.Has.Attempt = true }
func (t *ToolCall) SetToolName(v string)  { t.ToolName = v; t.ensureHas(); t.Has.ToolName = true }
func (t *ToolCall) SetToolKind(v string)  { t.ToolKind = v; t.ensureHas(); t.Has.ToolKind = true }
func (t *ToolCall) SetStatus(v string)    { t.Status = v; t.ensureHas(); t.Has.Status = true }
