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
	msgID := uuid.NewString()
	ctx = context.WithValue(ctx, memory.ModelMessageIDKey, msgID)
	turn, ok := memory.TurnMetaFromContext(ctx)
	if !ok {
		turn = memory.TurnMeta{}
	}

	//TODO would it make sense to combine message, model call, payload in one call
	if turn.ConversationID != "" {
		one := 1
		o.r.RecordMessage(ctx, memory.Message{ID: msgID, ParentID: turn.ParentMessageID, ConversationID: turn.ConversationID, Role: "assistant", Content: string(info.Payload), CreatedAt: time.Now(), Interim: &one})
	}
	o.r.StartModelCall(ctx, rec.ModelCallStart{MessageID: msgID, TurnID: turn.TurnID, Provider: info.Provider, Model: info.Model, ModelKind: info.ModelKind, StartedAt: o.start.StartedAt, Request: info.RequestJSON})
	return ctx
}

func (o *recorderObserver) OnCallEnd(ctx context.Context, info Info) {
	if !o.hasBeg { // tolerate missing start
		o.start = Info{}
	}
	turn, ok := memory.TurnMetaFromContext(ctx)
	if !ok {
		turn = memory.TurnMeta{}
	}
	// attach to message/turn from context
	msgID := memory.ModelMessageIDFromContext(ctx)
	if msgID == "" {
		return
	}

	//TODO would it make sense to combine message, model call, payload in one call
	if info.LLMResponse != nil {
		interim := 1
		o.r.RecordMessage(ctx, memory.Message{ID: msgID, ConversationID: turn.ConversationID, Actor: "planner", Interim: &interim})
	}
	// Finish model call first
	o.r.FinishModelCall(ctx, rec.ModelCallFinish{MessageID: msgID, TurnID: turn.TurnID, Usage: info.Usage, FinishReason: info.FinishReason, Cost: info.Cost, CompletedAt: info.CompletedAt, Response: info.ResponseJSON})
}

// WithRecorderObserver injects a recorder-backed Observer into context.
func WithRecorderObserver(ctx context.Context, r rec.Recorder) context.Context {
	_, ok := memory.TurnMetaFromContext(ctx) //ensure turn is in context
	if !ok {
		ctx = memory.WithTurnMeta(ctx, memory.TurnMeta{
			TurnID:          uuid.New().String(),
			ConversationID:  memory.ConversationIDFromContext(ctx),
			ParentMessageID: memory.ModelMessageIDFromContext(ctx),
		})
	}
	return WithObserver(ctx, &recorderObserver{r: r})
}
