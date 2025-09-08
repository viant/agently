package recorder

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/redact"
	"github.com/viant/agently/internal/dao/factory"
	daofactory "github.com/viant/agently/internal/dao/factory"
	msgw "github.com/viant/agently/internal/dao/message/write"
	mcw "github.com/viant/agently/internal/dao/modelcall/write"
	plw "github.com/viant/agently/internal/dao/payload/write"
	tcw "github.com/viant/agently/internal/dao/toolcall/write"
	turnw "github.com/viant/agently/internal/dao/turn/write"
	usagew "github.com/viant/agently/internal/dao/usage/write"
	d "github.com/viant/agently/internal/domain"
	storeadapter "github.com/viant/agently/internal/domain/adapter"
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
	RecordMessage(ctx context.Context, m memory.Message)
}

// TurnRecorder persists turn lifecycle events.
type TurnRecorder interface {
	StartTurn(ctx context.Context, conversationID, turnID string, at time.Time)
	UpdateTurn(ctx context.Context, turnID, status string)
}

// ToolCallRecorder persists tool-call operations (with optional payloads and metadata).
type ToolCallRecorder interface {
	StartToolCall(ctx context.Context, start ToolCallStart)
	FinishToolCall(ctx context.Context, upd ToolCallUpdate)
}

// Add a new function RecordUpdateToolStatus(ctx context.Context, messageID, completedAt time.Time, errMsg string, response interface{})
// ModelCallRecorder persists model-call operations (with optional payloads and metadata).
type ModelCallRecorder interface {
	StartModelCall(ctx context.Context, start ModelCallStart)
	FinishModelCall(ctx context.Context, finish ModelCallFinish)
}

type ModelCallStart struct {
	MessageID string
	TurnID    string
	Provider  string
	Model     string
	ModelKind string
	StartedAt time.Time
	Request   interface{}
}

type ModelCallFinish struct {
	MessageID    string
	TurnID       string
	Usage        *llm.Usage
	FinishReason string
	Cost         *float64
	CompletedAt  time.Time
	Response     interface{}
}

// UsageRecorder persists usage totals aggregated per conversation.
type UsageRecorder interface {
	RecordUsageTotals(ctx context.Context, conversationID string, input, output, embed int)
}

// Recorder is the unified surface that composes the smaller responsibilities.
// Downstream code can depend on individual sub-interfaces to reduce coupling
// and enable plugging alternative implementations (e.g. history DAO, exec traces).
type Recorder interface {
	Enablement
	MessageRecorder
	TurnRecorder
	ToolCallRecorder
	ModelCallRecorder
	UsageRecorder
}

// Writer is kept as a backward-compatible alias for Recorder.
// Deprecated: prefer depending on specific sub-interfaces or Recorder.
type Writer = Recorder

// Compile-time assertions: Store implements all recorder facets.
var _ Enablement = (*Store)(nil)
var _ MessageRecorder = (*Store)(nil)
var _ TurnRecorder = (*Store)(nil)
var _ ToolCallRecorder = (*Store)(nil)
var _ ModelCallRecorder = (*Store)(nil)
var _ UsageRecorder = (*Store)(nil)
var _ Recorder = (*Store)(nil)

type Store struct {
	mode  Mode
	store d.Store
}

func (w *Store) Enabled() bool {
	return w != nil && w.store != nil && (w.mode == ModeShadow || w.mode == ModeFull)
}

func (w *Store) RecordMessage(ctx context.Context, m memory.Message) {
	if !w.Enabled() {
		return
	}
	id := m.ID
	if id == "" {
		id = uuid.New().String()
	}
	rec := &msgw.Message{Id: id, Has: &msgw.MessageHas{Id: true}}
	if m.ConversationID != "" {
		rec.SetConversationID(m.ConversationID)
	}
	if turn := memory.TurnIDFromContext(ctx); turn != "" {
		rec.SetTurnID(turn)
	}
	if m.ParentID != "" {
		rec.SetParentMessageID(m.ParentID)
	}
	if m.Role != "" {
		rec.SetRole(m.Role)
	}
	// memory.Message has no Type; default to text
	rec.SetType("text")
	if m.Content != "" {
		if len(m.Content) > 65535 {
			fmt.Printf("WARN### Recorder.RecordMessage: content size %dB exceeds 65535; message may be truncated or rejected\n", len(m.Content))
		}
		rec.SetContent(m.Content)
	}

	var elicitationRec *plw.Payload
	var elicitationPayloadID string

	if m.Elicitation != nil {
		// Persist elicitation payload and link via ElicitationID.
		// Use payload ID as canonical elicitation_id so POST/lookup can round-trip reliably.
		pid := uuid.New().String()
		// Ensure the payload body carries the same opaque ID.
		m.Elicitation.ElicitationId = pid
		if b := toJSONBytes(m.Elicitation); len(b) > 0 {
			elicitationRec = &plw.Payload{Id: pid, Has: &plw.PayloadHas{Id: true}}
			elicitationRec.SetKind("elicitation_request")
			elicitationRec.SetMimeType("application/json")
			elicitationRec.SetSizeBytes(len(b))
			elicitationRec.SetStorage("inline")
			elicitationRec.SetInlineBody(b)
			elicitationRec.SetCompression("none")
			elicitationPayloadID = pid
		}
	}

	if m.Interim != nil && *m.Interim == 1 {
		one := 1
		rec.Interim = &one
		if rec.Has == nil {
			rec.Has = &msgw.MessageHas{}
		}
		rec.Has.Interim = true
		rec.Content = "TODO content available in payload" // TODO Placeholder until we support payloads for messages
		rec.Has.Content = true
	}

	if m.ToolName != nil {
		rec.SetToolName(*m.ToolName)
	}
	if !m.CreatedAt.IsZero() {
		rec.SetCreatedAt(m.CreatedAt)
	}

	if elicitationRec != nil {
		if _, err := w.store.Payloads().Patch(ctx, elicitationRec); err == nil {
			// Store payload ID as message.elicitation_id for consistent matching on POST.
			if elicitationPayloadID != "" {
				rec.SetElicitationID(elicitationPayloadID)
			}
		} else {
			fmt.Printf("ERROR### Recorder.RecordMessage elicitation: %v\n", err)
		}
	}
	if _, err := w.store.Messages().Patch(ctx, rec); err != nil {
		fmt.Printf("ERROR### Recorder.RecordMessage: %v\n", err)
	}

}

func (w *Store) StartTurn(ctx context.Context, conversationID, turnID string, at time.Time) {
	if !w.Enabled() || conversationID == "" {
		return
	}
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
		fmt.Printf("ERROR### Recorder.StartTurn: %v\n", err)
	}
}

func (w *Store) UpdateTurn(ctx context.Context, turnID, status string) {
	if !w.Enabled() || turnID == "" || status == "" {
		return
	}
	rec := &turnw.Turn{Has: &turnw.TurnHas{}}
	rec.SetId(turnID)
	rec.SetStatus(status)
	if err := w.store.Turns().Update(ctx, rec); err != nil {
		fmt.Printf("ERROR### Recorder.UpdateTurn: %v\n", err)
	}
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
func (w *Store) persistToolRequestPayload(ctx context.Context, request map[string]interface{}) (reqID string) {
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
		if _, err := w.store.Payloads().Patch(ctx, pw); err == nil {
			reqID = id
		} else {
			fmt.Printf("ERROR### Recorder.persistToolRequestPayload: %v\n", err)
		}
	}
	return reqID
}

// persistToolResponsePayload persists the response payload and returns its ID.
func (w *Store) persistToolResponsePayload(ctx context.Context, response interface{}) (resID string) {
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
		if _, err := w.store.Payloads().Patch(ctx, pw); err == nil {
			resID = id
		} else {
			fmt.Printf("ERROR### Recorder.persistToolResponsePayload: %v\n", err)
		}
	}
	return resID
}

// StartToolCall persists the initial request and metadata.
func (w *Store) StartToolCall(ctx context.Context, start ToolCallStart) {
	if !w.Enabled() || start.MessageID == "" || start.ToolName == "" {
		return
	}
	// Defer request payload persistence to FinishToolCall where we persist
	// both request and response and reference them via payloadId snapshots.
}

// FinishToolCall updates status and persists the response.
func (w *Store) FinishToolCall(ctx context.Context, upd ToolCallUpdate) {
	if !w.Enabled() || upd.MessageID == "" || upd.ToolName == "" || upd.Status == "" {
		return
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
		if id := w.persistToolRequestPayload(ctx, reqMap); id != "" {
			ref := `{"payloadId":"` + id + `"}`
			tw.RequestSnapshot = &ref
			if tw.Has == nil {
				tw.Has = &tcw.ToolCallHas{}
			}
			tw.Has.RequestSnapshot = true
		}
	}
	if upd.Response != nil {
		if id := w.persistToolResponsePayload(ctx, upd.Response); id != "" {
			ref := `{"payloadId":"` + id + `"}`
			tw.ResponseSnapshot = &ref
			if tw.Has == nil {
				tw.Has = &tcw.ToolCallHas{}
			}
			tw.Has.ResponseSnapshot = true
		}
	}
	if err := w.store.Operations().RecordToolCall(ctx, tw, "", ""); err != nil {
		fmt.Printf("ERROR### Recorder.FinishToolCall: %v\n", err)
	}
}

// RecordToolCall has been replaced by StartToolCall/FinishToolCall.

func (w *Store) RecordUsageTotals(ctx context.Context, conversationID string, input, output, embed int) {
	if !w.Enabled() || conversationID == "" {
		return
	}
	rec := &usagew.Usage{Has: &usagew.UsageHas{}}
	rec.SetConversationID(conversationID)
	rec.SetUsageInputTokens(input)
	rec.SetUsageOutputTokens(output)
	rec.SetUsageEmbeddingTokens(embed)
	if _, err := w.store.Usage().Patch(ctx, rec); err != nil {
		fmt.Printf("ERROR### Recorder.RecordUsageTotals: %v\n", err)
	}
}

// Deprecated RecordModelCall removed; use StartModelCall and FinishModelCall instead.

func (w *Store) StartModelCall(ctx context.Context, start ModelCallStart) {
	if !w.Enabled() || start.MessageID == "" || start.Model == "" {
		return
	}
	provider := start.Provider
	if provider == "" {
		provider = "unknown"
	}
	modelKind := start.ModelKind
	if modelKind == "" {
		modelKind = "chat"
	}
	var reqID string
	if rb := toJSONBytes(start.Request); len(rb) > 0 {
		b := redact.ScrubJSONBytes(rb, nil)
		id := uuid.New().String()
		pw := &plw.Payload{Id: id, Has: &plw.PayloadHas{Id: true}}
		pw.SetKind("model_request")
		pw.SetMimeType("application/json")
		pw.SetSizeBytes(len(b))
		pw.SetStorage("inline")
		pw.SetInlineBody(b)
		pw.SetCompression("none")
		if _, err := w.store.Payloads().Patch(ctx, pw); err == nil {
			reqID = id
		} else {
			fmt.Printf("ERROR### Recorder.StartModelCall payload: %v\n", err)
		}
	}
	mw := &mcw.ModelCall{}
	mw.SetMessageID(start.MessageID)
	mw.TurnID = strp(start.TurnID)
	mw.Has = &mcw.ModelCallHas{}
	mw.Has.TurnID = mw.TurnID != nil
	mw.SetProvider(provider)
	mw.SetModel(start.Model)
	mw.SetModelKind(modelKind)
	if !start.StartedAt.IsZero() {
		t := start.StartedAt
		mw.StartedAt = &t
		mw.Has.StartedAt = true
	}
	if err := w.store.Operations().RecordModelCall(ctx, mw, reqID, ""); err != nil {
		fmt.Printf("ERROR### StartModelCall: %v\n", err)
	}
}

func (w *Store) FinishModelCall(ctx context.Context, finish ModelCallFinish) {
	if !w.Enabled() || finish.MessageID == "" {
		return
	}
	var resID string
	if rb := toJSONBytes(finish.Response); len(rb) > 0 {
		b := redact.ScrubJSONBytes(rb, nil)
		id := uuid.New().String()
		pw := &plw.Payload{Id: id, Has: &plw.PayloadHas{Id: true}}
		pw.SetKind("model_response")
		pw.SetMimeType("application/json")
		pw.SetSizeBytes(len(b))
		pw.SetStorage("inline")
		pw.SetInlineBody(b)
		pw.SetCompression("none")
		if _, err := w.store.Payloads().Patch(ctx, pw); err == nil {
			resID = id
		} else {
			fmt.Printf("ERROR### Recorder.FinishModelCall payload: %v\n", err)
		}
	}
	var pt, ct, tt *int
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
		}
	}
	mw := &mcw.ModelCall{}
	mw.SetMessageID(finish.MessageID)
	mw.TurnID = strp(finish.TurnID)
	mw.Has = &mcw.ModelCallHas{}
	mw.Has.TurnID = mw.TurnID != nil
	if finish.FinishReason != "" {
		fr := finish.FinishReason
		mw.FinishReason = &fr
		mw.Has.FinishReason = true
	}
	if finish.Cost != nil {
		mw.Cost = finish.Cost
		mw.Has.Cost = true
	}
	if !finish.CompletedAt.IsZero() {
		t := finish.CompletedAt
		mw.CompletedAt = &t
		mw.Has.CompletedAt = true
	}
	if pt != nil {
		mw.PromptTokens = pt
		mw.Has.PromptTokens = true
	}
	if ct != nil {
		mw.CompletionTokens = ct
		mw.Has.CompletionTokens = true
	}
	if tt != nil {
		mw.TotalTokens = tt
		mw.Has.TotalTokens = true
	}
	if err := w.store.Operations().RecordModelCall(ctx, mw, "", resID); err != nil {
		fmt.Printf("ERROR### FinishModelCall: %v\n", err)
	}
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
	st := storeadapter.New(apis.Conversation, apis.Message, apis.Turn, apis.ModelCall, apis.ToolCall, apis.Payload, apis.Usage)
	return &Store{mode: mode, store: st}
}

func strp(s string) *string {
	if s != "" {
		return &s
	}
	return nil
}
