package recorder

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/viant/agently/genai/io/redact"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/memory"
	authctx "github.com/viant/agently/internal/auth"
	"github.com/viant/agently/internal/dao/factory"
	daofactory "github.com/viant/agently/internal/dao/factory"
	mcw "github.com/viant/agently/internal/dao/modelcall/write"
	plw "github.com/viant/agently/internal/dao/payload/write"
	tcw "github.com/viant/agently/internal/dao/toolcall/write"
	turnw "github.com/viant/agently/internal/dao/turn/write"
	d "github.com/viant/agently/internal/domain"
	storeadapter "github.com/viant/agently/internal/domain/adapter"
	msgw "github.com/viant/agently/pkg/agently/message"
	"github.com/viant/datly"
	"github.com/viant/datly/view"
)

type Mode string

const (
	ModeOff    Mode = "off"
	ModeShadow Mode = "shadow"
	ModeFull   Mode = "full"
)

// Enablement exposes a simple toggle to guard writes based on mode.
type Enablement interface {
	Enabled() bool
}

// MessageRecorder persists messages.
type MessageRecorder interface {
	RecordMessage(ctx context.Context, m memory.Message) error
}

// TurnRecorder persists turn lifecycle events.
type TurnRecorder interface {
	StartTurn(ctx context.Context, conversationID, turnID string, at time.Time) error
	UpdateTurn(ctx context.Context, turnID, status string) error
}

// ToolCallRecorder persists tool-call operations (with optional payloads and metadata).
type ToolCallRecorder interface {
	StartToolCall(ctx context.Context, start ToolCallStart) error
	FinishToolCall(ctx context.Context, upd ToolCallUpdate) error
}

// Add a new function RecordUpdateToolStatus(ctx context.Context, messageID, completedAt time.Time, errMsg string, response interface{})
// ModelCallRecorder persists model-call operations (with optional payloads and metadata).
type ModelCallRecorder interface {
	StartModelCall(ctx context.Context, start ModelCallStart) error
	FinishModelCall(ctx context.Context, finish ModelCallFinish) error
	AppendStreamChunk(ctx context.Context, payloadID string, chunk []byte) error
}

type ModelCallStart struct {
	MessageID       string
	TurnID          string
	Provider        string
	Model           string
	ModelKind       string
	StartedAt       time.Time
	Request         interface{}
	ProviderRequest interface{}
	StreamPayloadID string
}

type ModelCallFinish struct {
	MessageID        string
	TurnID           string
	Usage            *llm.Usage
	FinishReason     string
	Cost             *float64
	CompletedAt      time.Time
	Response         interface{}
	ProviderResponse interface{}
	StreamText       string
	StreamPayloadID  *string
	Status           string
}

// Recorder is the unified surface that composes the smaller responsibilities.
// Downstream code can depend on individual sub-interfaces to reduce coupling
// and enable plugging alternative implementations (e.g. history DAO, exec traces).
type Recorder interface {
	MessageRecorder
	TurnRecorder
	ToolCallRecorder
	ModelCallRecorder
}

// Writer is kept as a backward-compatible alias for Recorder.
// Deprecated: prefer depending on specific sub-interfaces or Recorder.
type Writer = Recorder

var _ MessageRecorder = (*Store)(nil)
var _ TurnRecorder = (*Store)(nil)
var _ ToolCallRecorder = (*Store)(nil)
var _ ModelCallRecorder = (*Store)(nil)
var _ Recorder = (*Store)(nil)

type Store struct {
	mode  Mode
	store d.Store
}

func (w *Store) RecordMessage(ctx context.Context, m memory.Message) error {
	if ctx == nil {
		return fmt.Errorf("record message: nil context")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("record message: %w", err)
	}
	id := m.ID
	if id == "" {
		id = uuid.New().String()
	}
	rec := &msgw.Message{Id: id, Has: &msgw.MessageHas{Id: true}}
	if m.ConversationID != "" {
		rec.SetConversationID(m.ConversationID)
	}
	if turn, ok := memory.TurnMetaFromContext(ctx); ok {
		rec.SetTurnID(turn.TurnID)
	}
	if m.ParentID != "" {
		rec.SetParentMessageID(m.ParentID)
	}
	if m.Role != "" {
		rec.SetRole(m.Role)
	}
	// memory.Messages has no Type; default to text
	rec.SetType("text")
	if m.Content != "" && (m.Interim == nil || *m.Interim == 0) {
		rec.SetContent(m.Content)
	}

	if m.Elicitation != nil {
		// Ensure the payload body carries the same opaque ID.
		if b := toJSONBytes(m.Elicitation); len(b) > 0 {
			rec.SetContent(string(b))
			rec.SetElicitationID(m.Elicitation.ElicitationId)
		}
	}

	if m.Interim != nil && *m.Interim == 1 {
		one := 1
		rec.Interim = &one
		if rec.Has == nil {
			rec.Has = &msgw.MessageHas{}
		}
		rec.Has.Interim = true
		rec.Has.Content = true
	}

	if m.ToolName != nil {
		rec.SetToolName(*m.ToolName)
	}
	if !m.CreatedAt.IsZero() {
		rec.SetCreatedAt(m.CreatedAt)
	}

	// Attach created_by_user_id from context when available
	if ui := authctx.User(ctx); ui != nil {
		userID := strings.TrimSpace(ui.Subject)
		if userID == "" {
			userID = strings.TrimSpace(ui.Email)
		}
		if userID != "" {
			rec.SetCreatedByUserID(userID)
		}
	}

	if _, err := w.store.Messages().Patch(ctx, rec); err != nil {
		return fmt.Errorf("record message: patch message: %w", err)
	}
	return nil
}

func (w *Store) StartTurn(ctx context.Context, conversationID, turnID string, at time.Time) error {
	if turnID == "" {
		turnID = uuid.New().String()
	}
	rec := &turnw.Turn{Has: &turnw.TurnHas{}}
	rec.SetId(turnID)
	rec.SetConversationID(conversationID)
	rec.SetStatus("running")
	if !at.IsZero() {
		rec.SetCreatedAt(at)
	}
	if _, err := w.store.Turns().Start(ctx, rec); err != nil {
		return err
	}
	return nil
}

func (w *Store) UpdateTurn(ctx context.Context, turnID, status string) error {
	rec := &turnw.Turn{Has: &turnw.TurnHas{}}
	rec.SetId(turnID)
	rec.SetStatus(status)
	if err := w.store.Turns().Update(ctx, rec); err != nil {
		return err
	}
	return nil
}

// ToolCallStart represents the initial tool-call data captured at start.
type ToolCallStart struct {
	MessageID string
	TurnID    string
	ToolName  string
	StartedAt time.Time
	Request   map[string]interface{}
}

// ToolCallUpdate represents the fields updated upon completion.
type ToolCallUpdate struct {
	MessageID   string
	TurnID      string
	ToolName    string
	Status      string
	CompletedAt time.Time
	ErrMsg      string
	Cost        *float64
	Response    interface{}
	// New fields to properly persist unique tool call rows
	ToolMessageID string
	OpID          string
	Attempt       int
	StartedAt     time.Time
	Request       interface{}
}

// persistToolRequestPayload persists the request payload and returns its ID.
func (w *Store) persistToolRequestPayload(ctx context.Context, request map[string]interface{}) (reqID string, err error) {
	if b := toJSONBytes(request); len(b) > 0 {
		sb := redact.ScrubJSONBytes(b, nil)
		id := uuid.New().String()
		pw := &plw.Payload{Id: id, Has: &plw.PayloadHas{Id: true}}
		pw.SetKind("tool_request")
		pw.SetMimeType("application/json")
		pw.SetSizeBytes(len(sb))
		pw.SetStorage("inline")
		pw.SetInlineBody(sb)
		pw.SetCompression("none")
		if _, err = w.store.Payloads().Patch(ctx, pw); err == nil {
			reqID = id
		}
	}
	return reqID, err
}

// persistToolResponsePayload persists the response payload and returns its ID.
func (w *Store) persistToolResponsePayload(ctx context.Context, response interface{}) (resID string, err error) {
	if b := toJSONBytes(response); len(b) > 0 {
		sb := redact.ScrubJSONBytes(b, nil)
		id := uuid.New().String()
		pw := &plw.Payload{Id: id, Has: &plw.PayloadHas{Id: true}}
		pw.SetKind("tool_response")
		pw.SetMimeType("application/json")
		pw.SetSizeBytes(len(sb))
		pw.SetStorage("inline")
		pw.SetInlineBody(sb)
		pw.SetCompression("none")
		if _, err = w.store.Payloads().Patch(ctx, pw); err == nil {
			resID = id
		}
	}
	return resID, err
}

// StartToolCall persists the initial request and metadata.
func (w *Store) StartToolCall(ctx context.Context, start ToolCallStart) error {
	if start.MessageID == "" || start.ToolName == "" {
		return nil
	}
	// Defer request payload persistence to FinishToolCall where we persist
	// both request and response and reference them via payloadId snapshots.
	return nil
}

// FinishToolCall updates status and persists the response.
func (w *Store) FinishToolCall(ctx context.Context, upd ToolCallUpdate) error {
	if upd.MessageID == "" || upd.ToolName == "" || upd.Status == "" {
		return fmt.Errorf("invalid tool call update: messageID, toolName and status are required")
	}
	tw := &tcw.ToolCall{}
	// Use per-tool message id when provided; fallback to parent message id
	msgID := upd.ToolMessageID
	if strings.TrimSpace(msgID) == "" {
		msgID = upd.MessageID
	}
	tw.SetMessageID(msgID)
	if upd.TurnID != "" {
		tw.TurnID = &upd.TurnID
		if tw.Has == nil {
			tw.Has = &tcw.ToolCallHas{}
		}
		tw.Has.TurnID = true
	}
	// Identify op and attempt
	opID := strings.TrimSpace(upd.OpID)
	if opID == "" {
		opID = uuid.New().String()
	}
	tw.SetOpID(opID)
	att := upd.Attempt
	if att <= 0 {
		att = 1
	}
	tw.SetAttempt(att)

	// Required fields
	tw.SetToolName(upd.ToolName)
	tw.SetToolKind("general")
	tw.SetStatus(upd.Status)
	if !upd.CompletedAt.IsZero() {
		tw.CompletedAt = &upd.CompletedAt
		if tw.Has == nil {
			tw.Has = &tcw.ToolCallHas{}
		}
		tw.Has.CompletedAt = true
	}
	if !upd.StartedAt.IsZero() {
		tw.StartedAt = &upd.StartedAt
		if tw.Has == nil {
			tw.Has = &tcw.ToolCallHas{}
		}
		tw.Has.StartedAt = true
	}
	if upd.ErrMsg != "" {
		tw.ErrorMessage = &upd.ErrMsg
		if tw.Has == nil {
			tw.Has = &tcw.ToolCallHas{}
		}
		tw.Has.ErrorMessage = true
	}
	if upd.Cost != nil {
		tw.Cost = upd.Cost
		if tw.Has == nil {
			tw.Has = &tcw.ToolCallHas{}
		}
		tw.Has.Cost = true
	}
	// Persist payloads and reference by payloadId in snapshots
	if upd.Request != nil {
		// Request may be map[string]any or raw json; persist and reference
		var reqMap map[string]interface{}
		switch r := upd.Request.(type) {
		case map[string]interface{}:
			reqMap = r
		default:
			// try to marshal/unmarshal into map for consistent scrubbing
			if b := toJSONBytes(upd.Request); len(b) > 0 {
				_ = json.Unmarshal(b, &reqMap)
			}
		}
		if id, perr := w.persistToolRequestPayload(ctx, reqMap); id != "" {
			tw.RequestPayloadID = &id
			if tw.Has == nil {
				tw.Has = &tcw.ToolCallHas{}
			}
			tw.Has.RequestPayloadID = true
		} else if perr != nil {
			return perr
		}
	}
	if upd.Response != nil {
		if id, perr := w.persistToolResponsePayload(ctx, upd.Response); id != "" {
			tw.ResponsePayloadID = &id
			if tw.Has == nil {
				tw.Has = &tcw.ToolCallHas{}
			}
			tw.Has.ResponsePayloadID = true
		} else if perr != nil {
			return perr
		}
	}
	if err := w.store.Operations().RecordToolCall(ctx, tw); err != nil {
		return err
	}
	return nil
}

// Deprecated RecordModelCall removed; use StartModelCall and FinishModelCall instead.

func (w *Store) StartModelCall(ctx context.Context, start ModelCallStart) error {
	ctx2 := context.WithoutCancel(ctx)
	if start.MessageID == "" || start.Model == "" {
		return nil
	}
	provider := start.Provider
	if provider == "" {
		provider = "unknown"
	}
	modelKind := start.ModelKind
	if modelKind == "" {
		modelKind = "chat"
	}
	modelCAll := &mcw.ModelCall{}
	modelCAll.SetMessageID(start.MessageID)
	modelCAll.TurnID = strp(start.TurnID)
	modelCAll.Has = &mcw.ModelCallHas{}
	modelCAll.Has.TurnID = modelCAll.TurnID != nil
	modelCAll.SetProvider(provider)
	modelCAll.SetModel(start.Model)
	modelCAll.SetModelKind(modelKind)
	modelCAll.SetStatus("queued")
	if !start.StartedAt.IsZero() {
		t := start.StartedAt
		modelCAll.StartedAt = &t
		modelCAll.Has.StartedAt = true
	}

	if rb := toJSONBytes(start.Request); len(rb) > 0 {
		b := redact.ScrubJSONBytes(rb, nil)
		id := uuid.New().String()
		payload := &plw.Payload{Id: id, Has: &plw.PayloadHas{Id: true}}
		payload.SetKind("model_request")
		payload.SetMimeType("application/json")
		payload.SetSizeBytes(len(b))
		payload.SetStorage("inline")
		payload.SetInlineBody(b)
		payload.SetCompression("none")
		if _, err := w.store.Payloads().Patch(ctx2, payload); err != nil {
			return err
		} else {
			// attach to write model directly
			modelCAll.RequestPayloadID = &id
			if modelCAll.Has == nil {
				modelCAll.Has = &mcw.ModelCallHas{}
			}
			modelCAll.Has.RequestPayloadID = true
		}
	}
	// Persist provider-specific request payload (raw wire body)
	if prb := toJSONBytes(start.ProviderRequest); len(prb) > 0 {
		b := redact.ScrubJSONBytes(prb, nil)
		id := uuid.New().String()
		payload := &plw.Payload{Id: id, Has: &plw.PayloadHas{Id: true}}
		payload.SetKind("provider_request")
		payload.SetMimeType("application/json")
		payload.SetSizeBytes(len(b))
		payload.SetStorage("inline")
		payload.SetInlineBody(b)
		payload.SetCompression("none")
		if _, err := w.store.Payloads().Patch(ctx2, payload); err == nil {
			modelCAll.ProviderRequestPayloadID = &id
			if modelCAll.Has == nil {
				modelCAll.Has = &mcw.ModelCallHas{}
			}
			modelCAll.Has.ProviderRequestPayloadID = true
		} else {
			return err
		}
	}

	if err := w.store.Operations().RecordModelCall(ctx2, modelCAll); err != nil {
		return err
	}

	return nil
}

func (w *Store) FinishModelCall(ctx context.Context, finish ModelCallFinish) error {
	if finish.MessageID == "" {
		return nil
	}
	modelCall := &mcw.ModelCall{}
	modelCall.SetMessageID(finish.MessageID)
	modelCall.TurnID = strp(finish.TurnID)
	modelCall.Has = &mcw.ModelCallHas{}
	modelCall.Has.TurnID = modelCall.TurnID != nil
	modelCall.Status = finish.Status
	modelCall.Has.Status = true

	// TODO set the right status
	ctx2 := context.WithoutCancel(ctx)
	if rb := toJSONBytes(finish.Response); len(rb) > 0 {
		b := redact.ScrubJSONBytes(rb, nil)
		id := uuid.New().String()
		payload := &plw.Payload{Id: id, Has: &plw.PayloadHas{Id: true}}
		payload.SetKind("model_response")
		payload.SetMimeType("application/json")
		payload.SetSizeBytes(len(b))
		payload.SetStorage("inline")
		payload.SetInlineBody(b)
		payload.SetCompression("none")
		if _, err := w.store.Payloads().Patch(ctx2, payload); err != nil {
			return err
		} else {
			// attach to write model directly
			modelCall.ResponsePayloadID = &id
			if modelCall.Has == nil {
				modelCall.Has = &mcw.ModelCallHas{}
			}
			modelCall.Has.ResponsePayloadID = true
		}
	}
	// Persist provider-specific response payload (raw wire body)
	if prb := toJSONBytes(finish.ProviderResponse); len(prb) > 0 {
		b := redact.ScrubJSONBytes(prb, nil)
		id := uuid.New().String()
		payload := &plw.Payload{Id: id, Has: &plw.PayloadHas{Id: true}}
		payload.SetKind("provider_response")
		payload.SetMimeType("application/json")
		payload.SetSizeBytes(len(b))
		payload.SetStorage("inline")
		payload.SetInlineBody(b)
		payload.SetCompression("none")
		if _, err := w.store.Payloads().Patch(ctx2, payload); err == nil {
			modelCall.ProviderResponsePayloadID = &id
			if modelCall.Has == nil {
				modelCall.Has = &mcw.ModelCallHas{}
			}
			modelCall.Has.ProviderResponsePayloadID = true
		} else {
			return err
		}
	}
	// (stream payload created in StartModelCall)
	if finish.StreamPayloadID != nil && strings.TrimSpace(*finish.StreamPayloadID) != "" {
		modelCall.StreamPayloadID = finish.StreamPayloadID
		if modelCall.Has == nil {
			modelCall.Has = &mcw.ModelCallHas{}
		}
		modelCall.Has.StreamPayloadID = true
	} else if strings.TrimSpace(finish.StreamText) != "" {
		sb := []byte(finish.StreamText)
		id := uuid.New().String()
		payload := &plw.Payload{Id: id, Has: &plw.PayloadHas{Id: true}}
		payload.SetKind("model_stream")
		payload.SetMimeType("text/plain")
		payload.SetSizeBytes(len(sb))
		payload.SetStorage("inline")
		payload.SetInlineBody(sb)
		payload.SetCompression("none")
		if _, err := w.store.Payloads().Patch(ctx2, payload); err == nil {
			modelCall.StreamPayloadID = &id
			if modelCall.Has == nil {
				modelCall.Has = &mcw.ModelCallHas{}
			}
			modelCall.Has.StreamPayloadID = true
		} else {
			return err
		}
	}
	var pt, ct, tt *int
	var pCached, pAudio, cReason, cAudio, cAccPred, cRejPred *int
	if u := finish.Usage; u != nil {
		if u.PromptTokens > 0 {
			v := u.PromptTokens
			pt = &v
		}
		if u.CompletionTokens > 0 {
			v := u.CompletionTokens
			ct = &v
		}
		if u.TotalTokens > 0 {
			v := u.TotalTokens
			tt = &v
			// Fallback: if provider only reports total, attribute to completion tokens
			if pt == nil && ct == nil {
				vv := u.TotalTokens
				ct = &vv
			}
		}
		if u.PromptCachedTokens > 0 {
			v := u.PromptCachedTokens
			pCached = &v
		}
		if u.PromptAudioTokens > 0 {
			v := u.PromptAudioTokens
			pAudio = &v
		}
		if u.CompletionReasoningTokens > 0 {
			v := u.CompletionReasoningTokens
			cReason = &v
		}
		if u.CompletionAudioTokens > 0 {
			v := u.CompletionAudioTokens
			cAudio = &v
		}
		if u.AcceptedPredictionTokens > 0 { // maps to completion_* per OpenAI
			v := u.AcceptedPredictionTokens
			cAccPred = &v
		}
		if u.RejectedPredictionTokens > 0 {
			v := u.RejectedPredictionTokens
			cRejPred = &v
		}
	}
	if finish.FinishReason != "" {
		fr := finish.FinishReason
		modelCall.FinishReason = &fr
		modelCall.Has.FinishReason = true
	}
	if finish.Cost != nil {
		modelCall.Cost = finish.Cost
		modelCall.Has.Cost = true
	}
	if !finish.CompletedAt.IsZero() {
		t := finish.CompletedAt
		modelCall.CompletedAt = &t
		modelCall.Has.CompletedAt = true
	}
	if pt != nil {
		modelCall.PromptTokens = pt
		modelCall.Has.PromptTokens = true
	}
	if ct != nil {
		modelCall.CompletionTokens = ct
		modelCall.Has.CompletionTokens = true
	}
	if tt != nil {
		modelCall.TotalTokens = tt
		modelCall.Has.TotalTokens = true
	}
	if pCached != nil {
		modelCall.PromptCachedTokens = pCached
		modelCall.Has.PromptCachedTokens = true
	}
	if pAudio != nil {
		modelCall.PromptAudioTokens = pAudio
		modelCall.Has.PromptAudioTokens = true
	}
	if cReason != nil {
		modelCall.CompletionReasoningTokens = cReason
		modelCall.Has.CompletionReasoningTokens = true
	}
	if cAudio != nil {
		modelCall.CompletionAudioTokens = cAudio
		modelCall.Has.CompletionAudioTokens = true
	}
	if cAccPred != nil {
		modelCall.CompletionAcceptedPredictionTokens = cAccPred
		modelCall.Has.CompletionAcceptedPredictionTokens = true
	}
	if cRejPred != nil {
		modelCall.CompletionRejectedPredictionTokens = cRejPred
		modelCall.Has.CompletionRejectedPredictionTokens = true
	}
	if err := w.store.Operations().RecordModelCall(ctx2, modelCall); err != nil {
		return err
	}

	return nil
}

// AppendStreamChunk appends bytes to inline stream payload by id (best-effort).
func (w *Store) AppendStreamChunk(ctx context.Context, payloadID string, chunk []byte) error {
	ctx2 := context.WithoutCancel(ctx)
	if strings.TrimSpace(payloadID) == "" || len(chunk) == 0 {
		return nil
	}
	pv, err := w.store.Payloads().Get(ctx2, payloadID)
	if err != nil {
		return err
	}
	var cur []byte
	if pv != nil && pv.InlineBody != nil {
		cur = *pv.InlineBody
	}
	next := append(cur, chunk...)
	rec := &plw.Payload{Id: payloadID, Has: &plw.PayloadHas{Id: true}}
	rec.SetKind("model_stream")
	rec.SetMimeType("text/plain")
	rec.SetSizeBytes(len(next))
	rec.SetStorage("inline")
	rec.SetInlineBody(next)
	rec.SetCompression("none")
	if _, err := w.store.Payloads().Patch(ctx2, rec); err != nil {
		return err
	}
	return nil
}

func toJSONBytes(v interface{}) []byte {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case []byte:
		return t
	case json.RawMessage:
		return []byte(t)
	default:
		b, _ := json.Marshal(v)
		return b
	}
}

// New builds a store-backed Writer using in-memory DAO backends by default.
// When AGENTLY_DOMAIN_MODE is "off" it returns a disabled writer.
func New(ctx context.Context) Writer {
	mode := Mode(os.Getenv("AGENTLY_DOMAIN_MODE"))
	if mode == "" {
		// Default to shadow writes when not explicitly configured so
		// v1 endpoints can persist via DAO-backed store out of the box.
		mode = ModeShadow
	}
	if mode == ModeOff {
		return &Store{mode: ModeOff}
	}

	var apis *factory.API
	driver := strings.TrimSpace(os.Getenv("AGENTLY_DB_DRIVER"))
	dsn := strings.TrimSpace(os.Getenv("AGENTLY_DB_DSN"))

	// Prefer SQL-backed DAOs whenever a connector is configured, regardless of mode
	if driver != "" && dsn != "" {
		if dao, err := datly.New(ctx); err == nil {
			err = dao.AddConnectors(ctx, view.NewConnector("agently", driver, dsn))
			if err == nil {
				apis, _ = daofactory.New(ctx, daofactory.DAOSQL, dao)
			}
		}
	}

	if apis == nil {
		apis, _ = daofactory.New(ctx, daofactory.DAOInMemory, nil)
	}

	if apis == nil {
		return &Store{mode: ModeOff}
	}
	st := storeadapter.New(apis.Conversation, apis.Message, apis.Turn, apis.ModelCall, apis.ToolCall, apis.Payload)
	return &Store{mode: mode, store: st}
}

func strp(s string) *string {
	if s != "" {
		return &s
	}
	return nil
}
