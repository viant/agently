package modelcallctx

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/viant/agently/genai/memory"
	rec "github.com/viant/agently/internal/domain/recorder"
)

// recorderObserver implements Observer and writes model-call data directly to recorder.
type recorderObserver struct {
	r      rec.Recorder
	start  Info
	hasBeg bool
}

func (o *recorderObserver) OnCallStart(ctx context.Context, info Info) context.Context {
	o.start = info
	o.hasBeg = true
	if info.StartedAt.IsZero() {
		o.start.StartedAt = time.Now()
	}

	//TODO create a message call ,use parentMessageID  from memory.ModelMessageIDFromContext(ctx) (remove upstream assistant message creation if present)
	//TODO then RecordModelCall
	//TODO saturate new message ID in context so that OnCallEnd can patch right model_call with response_call_id,

	// Ensure a target message id for this call (create when absent) and write initial model_call row.
	msgID := memory.ModelMessageIDFromContext(ctx)
	if msgID == "" {
		// create a new message id for this model call
		msgID = uuid.NewString()
		ctx = context.WithValue(ctx, memory.ModelMessageIDKey, msgID)
	}
	turnID := memory.TurnIDFromContext(ctx)
	if tm, ok := memory.TurnMetaFromContext(ctx); ok {
		if tm.TurnID != "" {
			turnID = tm.TurnID
		}
	}
	if msgID != "" && o.r != nil && o.r.Enabled() {
		// Create assistant message with parent/conversation from TurnMeta
		convID := memory.ConversationIDFromContext(ctx)
		parentID := memory.MessageIDFromContext(ctx)
		if tm, ok := memory.TurnMetaFromContext(ctx); ok {
			if tm.ConversationID != "" {
				convID = tm.ConversationID
			}
			if tm.ParentMessageID != "" {
				parentID = tm.ParentMessageID
			}
		}
		if convID != "" {
			o.r.RecordMessage(ctx, memory.Message{ID: msgID, ParentID: parentID, ConversationID: convID, Role: "assistant", Content: " ", CreatedAt: time.Now()})
		}
		o.r.StartModelCall(ctx, rec.ModelCallStart{MessageID: msgID, TurnID: turnID, Provider: info.Provider, Model: info.Model, ModelKind: info.ModelKind, StartedAt: o.start.StartedAt, Request: info.RequestJSON})
	}

	return ctx
}

func (o *recorderObserver) OnCallEnd(ctx context.Context, info Info) {
	if !o.hasBeg { // tolerate missing start
		o.start = Info{}
	}

	//Match model call baased on saturated message id (created in OnCallStart)
	//RecordModelCall with response rson and completed time

	// merge fields
	if len(info.RequestJSON) == 0 {
		info.RequestJSON = o.start.RequestJSON
	}
	if info.StartedAt.IsZero() {
		info.StartedAt = o.start.StartedAt
	}
	// attach to message/turn from context
	msgID := memory.ModelMessageIDFromContext(ctx)
	if msgID == "" {
		msgID = memory.MessageIDFromContext(ctx)
	}
	turnID := memory.TurnIDFromContext(ctx)
	if tm, ok := memory.TurnMetaFromContext(ctx); ok {
		if tm.TurnID != "" {
			turnID = tm.TurnID
		}
	}
	if msgID == "" || o.r == nil || !o.r.Enabled() {
		return
	}
	o.r.FinishModelCall(ctx, rec.ModelCallFinish{MessageID: msgID, TurnID: turnID, Usage: info.Usage, FinishReason: info.FinishReason, Cost: info.Cost, CompletedAt: info.CompletedAt, Response: info.ResponseJSON})
}

// WithRecorderObserver injects a recorder-backed Observer into context.
func WithRecorderObserver(ctx context.Context, r rec.Recorder) context.Context {
	return WithObserver(ctx, &recorderObserver{r: r})
}
