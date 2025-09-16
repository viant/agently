package modelcallctx

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/viant/agently/genai/memory"
	rec "github.com/viant/agently/internal/domain/recorder"
)

// recorderObserver implements Observer and writes model-call data directly to recorder.
type recorderObserver struct {
	r               rec.Recorder
	start           Info
	hasBeg          bool
	acc             strings.Builder
	streamPayloadID string
}

func (o *recorderObserver) OnCallStart(ctx context.Context, info Info) (context.Context, error) {
	o.start = info
	o.hasBeg = true
	if info.StartedAt.IsZero() {
		o.start.StartedAt = time.Now()
	}
	// Attach finish barrier so downstream can wait for persistence before emitting final message.
	ctx, _ = WithFinishBarrier(ctx)
	msgID := uuid.NewString()
	ctx = context.WithValue(ctx, memory.ModelMessageIDKey, msgID)
	turn, ok := memory.TurnMetaFromContext(ctx)
	if !ok {
		turn = memory.TurnMeta{}
	}

	//TODO would it make sense to combine message, model call, payload in one call
	if turn.ConversationID != "" {
		one := 1
		if err := o.r.RecordMessage(ctx, memory.Message{ID: msgID, ParentID: turn.ParentMessageID, ConversationID: turn.ConversationID, Role: "assistant", Content: string(info.Payload), CreatedAt: time.Now(), Interim: &one}); err != nil {
			return nil, err
		}
	}
	// Generate stream payload id up-front so deltas can append progressively
	o.streamPayloadID = uuid.New().String()

	if err := o.r.StartModelCall(ctx, rec.ModelCallStart{MessageID: msgID, TurnID: turn.TurnID, Provider: info.Provider, Model: info.Model, ModelKind: info.ModelKind, StartedAt: o.start.StartedAt, Request: info.Payload, ProviderRequest: info.RequestJSON, StreamPayloadID: o.streamPayloadID}); err != nil {
		return nil, err
	}
	// Capture stream payload id by reading it from model_calls row is expensive; rely on recorder contract
	// to seed it in StartModelCall and use AppendStreamChunk via payload id carried in observer state
	// For simplicity, we store it as messageID-derived mapping (not implemented). The recorder provides only
	// AppendStreamChunk by payload id, we cannot inspect it here without extra DAO read. We'll accumulate text
	// in OnStreamDelta and persist on Finish.
	return ctx, nil
}

func (o *recorderObserver) OnCallEnd(ctx context.Context, info Info) error {
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
		return nil
	}

	//TODO would it make sense to combine message, model call, payload in one call
	if info.LLMResponse != nil {
		interim := 1
		if err := o.r.RecordMessage(ctx, memory.Message{ID: msgID, ConversationID: turn.ConversationID, Actor: "planner", Interim: &interim}); err != nil {
			return err
		}
	}
	// Prefer provider-supplied stream text; fall back to accumulated chunks
	streamTxt := info.StreamText
	if strings.TrimSpace(streamTxt) == "" {
		streamTxt = o.acc.String()
	}

	status := "completed"
	if info.Err != "" {
		status = "failed"
	}

	// Finish model call first
	finishResult := rec.ModelCallFinish{
		MessageID:        msgID,
		TurnID:           turn.TurnID,
		Usage:            info.Usage,
		FinishReason:     info.FinishReason,
		Cost:             info.Cost,
		CompletedAt:      info.CompletedAt,
		Response:         info.LLMResponse,
		ProviderResponse: info.ResponseJSON,
		StreamText:       streamTxt,
		Status:           status,
	}
	if err := o.r.FinishModelCall(ctx, finishResult); err != nil {
		return err
	}

	// Signal finish so any waiters can proceed (e.g., emitting final assistant message)
	signalFinish(ctx)
	return nil
}

// OnStreamDelta aggregates streamed chunks. Persisted once in FinishModelCall.
func (o *recorderObserver) OnStreamDelta(_ context.Context, data []byte) {
	if len(data) == 0 {
		return
	}
	o.acc.Write(data)
	// Best-effort append to stream payload inline body
	if strings.TrimSpace(o.streamPayloadID) != "" && o.r != nil {
		_ = o.r.AppendStreamChunk(context.Background(), o.streamPayloadID, data)
	}
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
