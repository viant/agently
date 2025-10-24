package write

import "time"

var PackageName = "modelcall/write"

type ModelCall struct {
	MessageID                          string        `sqlx:"message_id,primaryKey" validate:"required"`
	TurnID                             *string       `sqlx:"turn_id" json:",omitempty"`
	Provider                           string        `sqlx:"provider" validate:"required"`
	Model                              string        `sqlx:"model" validate:"required"`
	ModelKind                          string        `sqlx:"model_kind" validate:"required"`
	Status                             string        `sqlx:"status" validate:"required"`
	ErrorCode                          *string       `sqlx:"error_code" json:",omitempty"`
	ErrorMessage                       *string       `sqlx:"error_message" json:",omitempty"`
	PromptTokens                       *int          `sqlx:"prompt_tokens" json:",omitempty"`
	PromptCachedTokens                 *int          `sqlx:"prompt_cached_tokens" json:",omitempty"`
	CompletionTokens                   *int          `sqlx:"completion_tokens" json:",omitempty"`
	TotalTokens                        *int          `sqlx:"total_tokens" json:",omitempty"`
	PromptAudioTokens                  *int          `sqlx:"prompt_audio_tokens" json:",omitempty"`
	CompletionReasoningTokens          *int          `sqlx:"completion_reasoning_tokens" json:",omitempty"`
	CompletionAudioTokens              *int          `sqlx:"completion_audio_tokens" json:",omitempty"`
	CompletionAcceptedPredictionTokens *int          `sqlx:"completion_accepted_prediction_tokens" json:",omitempty"`
	CompletionRejectedPredictionTokens *int          `sqlx:"completion_rejected_prediction_tokens" json:",omitempty"`
	FinishReason                       *string       `sqlx:"finish_reason" json:",omitempty"`
	StartedAt                          *time.Time    `sqlx:"started_at" json:",omitempty"`
	CompletedAt                        *time.Time    `sqlx:"completed_at" json:",omitempty"`
	LatencyMS                          *int          `sqlx:"latency_ms" json:",omitempty"`
	Cost                               *float64      `sqlx:"cost" json:",omitempty"`
	TraceID                            *string       `sqlx:"trace_id" json:",omitempty"`
	SpanID                             *string       `sqlx:"span_id" json:",omitempty"`
	RequestPayloadID                   *string       `sqlx:"request_payload_id" json:",omitempty"`
	ResponsePayloadID                  *string       `sqlx:"response_payload_id" json:",omitempty"`
	ProviderRequestPayloadID           *string       `sqlx:"provider_request_payload_id" json:",omitempty"`
	ProviderResponsePayloadID          *string       `sqlx:"provider_response_payload_id" json:",omitempty"`
	StreamPayloadID                    *string       `sqlx:"stream_payload_id" json:",omitempty"`
	Has                                *ModelCallHas `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

type ModelCallHas struct {
	MessageID                          bool
	TurnID                             bool
	Provider                           bool
	Model                              bool
	ModelKind                          bool
	ErrorCode                          bool
	ErrorMessage                       bool
	PromptTokens                       bool
	PromptCachedTokens                 bool
	CompletionTokens                   bool
	TotalTokens                        bool
	PromptAudioTokens                  bool
	CompletionReasoningTokens          bool
	CompletionAudioTokens              bool
	CompletionAcceptedPredictionTokens bool
	CompletionRejectedPredictionTokens bool
	FinishReason                       bool
	StartedAt                          bool
	CompletedAt                        bool
	LatencyMS                          bool
	Cost                               bool
	TraceID                            bool
	SpanID                             bool
	RequestPayloadID                   bool
	ResponsePayloadID                  bool
	ProviderRequestPayloadID           bool
	ProviderResponsePayloadID          bool
	StreamPayloadID                    bool
	Status                             bool
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
func (m *ModelCall) SetStatus(v string)    { m.Status = v; m.ensureHas(); m.Has.Status = true }
func (m *ModelCall) SetStreamPayloadID(v string) {
	m.StreamPayloadID = &v
	m.ensureHas()
	m.Has.StreamPayloadID = true
}

// Added convenience setters to avoid manual Has management across call sites.
func (m *ModelCall) SetTurnID(v string) { m.TurnID = &v; m.ensureHas(); m.Has.TurnID = true }
func (m *ModelCall) SetStartedAt(v time.Time) {
	m.StartedAt = &v
	m.ensureHas()
	m.Has.StartedAt = true
}
func (m *ModelCall) SetCompletedAt(v time.Time) {
	m.CompletedAt = &v
	m.ensureHas()
	m.Has.CompletedAt = true
}
func (m *ModelCall) SetPromptTokens(v int) {
	m.PromptTokens = &v
	m.ensureHas()
	m.Has.PromptTokens = true
}
func (m *ModelCall) SetCompletionTokens(v int) {
	m.CompletionTokens = &v
	m.ensureHas()
	m.Has.CompletionTokens = true
}
func (m *ModelCall) SetTotalTokens(v int) {
    m.TotalTokens = &v
    m.ensureHas()
    m.Has.TotalTokens = true
}
func (m *ModelCall) SetCost(v float64) {
    m.Cost = &v
    m.ensureHas()
    m.Has.Cost = true
}
func (m *ModelCall) SetRequestPayloadID(v string) {
	m.RequestPayloadID = &v
	m.ensureHas()
	m.Has.RequestPayloadID = true
}
func (m *ModelCall) SetResponsePayloadID(v string) {
	m.ResponsePayloadID = &v
	m.ensureHas()
	m.Has.ResponsePayloadID = true
}
func (m *ModelCall) SetProviderRequestPayloadID(v string) {
	m.ProviderRequestPayloadID = &v
	m.ensureHas()
	m.Has.ProviderRequestPayloadID = true
}
func (m *ModelCall) SetProviderResponsePayloadID(v string) {
	m.ProviderResponsePayloadID = &v
	m.ensureHas()
	m.Has.ProviderResponsePayloadID = true
}
func (m *ModelCall) SetErrorMessage(v string) {
	m.ErrorMessage = &v
	m.ensureHas()
	m.Has.ErrorMessage = true
}
func (m *ModelCall) SetErrorCode(v string) { m.ErrorCode = &v; m.ensureHas(); m.Has.ErrorCode = true }
