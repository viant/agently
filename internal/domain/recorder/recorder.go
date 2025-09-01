package recorder

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/redact"
	daofactory "github.com/viant/agently/internal/dao/factory"
	msgw "github.com/viant/agently/internal/dao/message/write"
	mcw "github.com/viant/agently/internal/dao/modelcall/write"
	plw "github.com/viant/agently/internal/dao/payload/write"
	tcw "github.com/viant/agently/internal/dao/toolcall/write"
	turnw "github.com/viant/agently/internal/dao/turn/write"
	usagew "github.com/viant/agently/internal/dao/usage/write"
	d "github.com/viant/agently/internal/domain"
	storeadapter "github.com/viant/agently/internal/domain/adapter"
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
	RecordTurnStart(ctx context.Context, conversationID, turnID string, at time.Time)
	RecordTurnUpdate(ctx context.Context, turnID, status string)
}

// ToolCallRecorder persists tool-call operations (with optional payloads and metadata).
type ToolCallRecorder interface {
	RecordToolCall(ctx context.Context, messageID, turnID, toolName, status string, startedAt, completedAt time.Time, errMsg string, cost *float64, request map[string]interface{}, response interface{})
}

// ModelCallRecorder persists model-call operations (with optional payloads and metadata).
type ModelCallRecorder interface {
	RecordModelCall(ctx context.Context, messageID, turnID, provider, model, modelKind string, usage *llm.Usage, finishReason string, cost *float64, startedAt, completedAt time.Time, request interface{}, response interface{})
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
	if m.ParentID != "" {
		rec.SetParentMessageID(m.ParentID)
	}
	if m.Role != "" {
		rec.SetRole(m.Role)
	}
	// memory.Message has no Type; default to text
	rec.SetType("text")
	if m.Content != "" {
		rec.SetContent(m.Content)
	}
	if m.ToolName != nil {
		rec.SetToolName(*m.ToolName)
	}
	if !m.CreatedAt.IsZero() {
		rec.SetCreatedAt(m.CreatedAt)
	}
	_, _ = w.store.Messages().Patch(ctx, rec)
}

func (w *Store) RecordTurnStart(ctx context.Context, conversationID, turnID string, at time.Time) {
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
	_, _ = w.store.Turns().Start(ctx, rec)
}

func (w *Store) RecordTurnUpdate(ctx context.Context, turnID, status string) {
	if !w.Enabled() || turnID == "" || status == "" {
		return
	}
	rec := &turnw.Turn{Has: &turnw.TurnHas{}}
	rec.SetId(turnID)
	rec.SetStatus(status)
	_ = w.store.Turns().Update(ctx, rec)
}

func (w *Store) RecordToolCall(ctx context.Context, messageID, turnID, toolName, status string, startedAt, completedAt time.Time, errMsg string, cost *float64, request map[string]interface{}, response interface{}) {
	if !w.Enabled() || messageID == "" || toolName == "" {
		return
	}
	// Persist sanitized request/response as payloads
	var reqID, resID string
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
		_, _ = w.store.Payloads().Patch(ctx, pw)
		reqID = id
	}
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
		_, _ = w.store.Payloads().Patch(ctx, pw)
		resID = id
	}
	// Build optional metadata
	var errPtr *string
	if strings := errMsg; strings != "" {
		errPtr = &errMsg
	}
	var startedPtr, completedPtr *time.Time
	if !startedAt.IsZero() {
		startedPtr = &startedAt
	}
	if !completedAt.IsZero() {
		completedPtr = &completedAt
	}
	tw := &tcw.ToolCall{}
	tw.SetMessageID(messageID)
	tw.TurnID = strp(turnID)
	if tw.TurnID != nil {
		if tw.Has == nil {
			tw.Has = &tcw.ToolCallHas{}
		}
		tw.Has.TurnID = true
	}
	tw.SetOpID(uuid.New().String())
	tw.SetAttempt(1)
	tw.SetToolName(toolName)
	tw.SetToolKind("general")
	tw.SetStatus(status)
	if startedPtr != nil {
		tw.StartedAt = startedPtr
		if tw.Has == nil {
			tw.Has = &tcw.ToolCallHas{}
		}
		tw.Has.StartedAt = true
	}
	if completedPtr != nil {
		tw.CompletedAt = completedPtr
		if tw.Has == nil {
			tw.Has = &tcw.ToolCallHas{}
		}
		tw.Has.CompletedAt = true
	}
	if errPtr != nil {
		tw.ErrorMessage = errPtr
		if tw.Has == nil {
			tw.Has = &tcw.ToolCallHas{}
		}
		tw.Has.ErrorMessage = true
	}
	if cost != nil {
		tw.Cost = cost
		if tw.Has == nil {
			tw.Has = &tcw.ToolCallHas{}
		}
		tw.Has.Cost = true
	}
	_ = w.store.Operations().RecordToolCall(ctx, tw, reqID, resID)
}

func (w *Store) RecordUsageTotals(ctx context.Context, conversationID string, input, output, embed int) {
	if !w.Enabled() || conversationID == "" {
		return
	}
	rec := &usagew.Usage{Has: &usagew.UsageHas{}}
	rec.SetConversationID(conversationID)
	rec.SetUsageInputTokens(input)
	rec.SetUsageOutputTokens(output)
	rec.SetUsageEmbeddingTokens(embed)
	_, _ = w.store.Usage().Patch(ctx, rec)
}

func (w *Store) RecordModelCall(ctx context.Context, messageID, turnID, provider, model, modelKind string, usage *llm.Usage, finishReason string, cost *float64, startedAt, completedAt time.Time, request interface{}, response interface{}) {
	if !w.Enabled() || messageID == "" || model == "" {
		return
	}
	if provider == "" {
		provider = "unknown"
	}
	if modelKind == "" {
		modelKind = "chat"
	}
	// Build payloads (inline JSON bodies)
	var reqID, resID string
	if rb := toJSONBytes(request); len(rb) > 0 {
		b := redact.ScrubJSONBytes(rb, nil)
		reqID = uuid.New().String()
		pw := &plw.Payload{Id: reqID, Has: &plw.PayloadHas{Id: true}}
		pw.SetKind("model_request")
		pw.SetMimeType("application/json")
		pw.SetSizeBytes(len(b))
		pw.SetStorage("inline")
		pw.SetInlineBody(b)
		pw.SetCompression("none")
		_, _ = w.store.Payloads().Patch(ctx, pw)
	}
	if rb := toJSONBytes(response); len(rb) > 0 {
		b := redact.ScrubJSONBytes(rb, nil)
		resID = uuid.New().String()
		pw := &plw.Payload{Id: resID, Has: &plw.PayloadHas{Id: true}}
		pw.SetKind("model_response")
		pw.SetMimeType("application/json")
		pw.SetSizeBytes(len(b))
		pw.SetStorage("inline")
		pw.SetInlineBody(b)
		pw.SetCompression("none")
		_, _ = w.store.Payloads().Patch(ctx, pw)
	}
	// pass payload IDs to operations
	// Optional usage/timing
	var pt, ct, tt *int
	if usage != nil {
		if usage.PromptTokens > 0 {
			v := usage.PromptTokens
			pt = &v
		}
		if usage.CompletionTokens > 0 {
			v := usage.CompletionTokens
			ct = &v
		}
		if usage.TotalTokens > 0 {
			v := usage.TotalTokens
			tt = &v
		}
	}
	var startedPtr, completedPtr *time.Time
	if !startedAt.IsZero() {
		startedPtr = &startedAt
	}
	if !completedAt.IsZero() {
		completedPtr = &completedAt
	}
	var frPtr *string
	if finishReason != "" {
		frPtr = &finishReason
	}
	mw := &mcw.ModelCall{}
	mw.SetMessageID(messageID)
	mw.TurnID = strp(turnID)
	if mw.TurnID != nil {
		mw.Has = &mcw.ModelCallHas{TurnID: true}
	} else {
		mw.Has = &mcw.ModelCallHas{}
	}
	mw.SetProvider(provider)
	mw.SetModel(model)
	mw.SetModelKind(modelKind)
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
	if frPtr != nil {
		mw.FinishReason = frPtr
		mw.Has.FinishReason = true
	}
	if cost != nil {
		mw.Cost = cost
		mw.Has.Cost = true
	}
	if startedPtr != nil {
		mw.StartedAt = startedPtr
		mw.Has.StartedAt = true
	}
	if completedPtr != nil {
		mw.CompletedAt = completedPtr
		mw.Has.CompletedAt = true
	}
	_ = w.store.Operations().RecordModelCall(ctx, mw, reqID, resID)
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
		mode = ModeOff
	}
	if mode == ModeOff {
		return &Store{mode: ModeOff}
	}
	apis, _ := daofactory.New(ctx, daofactory.DAOInMemory, nil)
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
