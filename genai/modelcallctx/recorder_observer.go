package modelcallctx

import (
	"context"
	"time"

	"github.com/viant/agently/genai/memory"
	rec "github.com/viant/agently/internal/domain/recorder"
)

// recorderObserver implements Observer and writes model-call data directly to recorder.
type recorderObserver struct {
	r      rec.Recorder
	start  Info
	hasBeg bool
}

func (o *recorderObserver) OnCallStart(ctx context.Context, info Info) {
	o.start = info
	o.hasBeg = true
	if info.StartedAt.IsZero() {
		o.start.StartedAt = time.Now()
	}
	// Write initial model_call row with started_at so status reads as scheduled.
	msgID := memory.MessageIDFromContext(ctx)
	turnID := memory.TurnIDFromContext(ctx)
	if msgID != "" && o.r != nil && o.r.Enabled() {
		o.r.RecordModelCall(ctx, msgID, turnID, info.Provider, info.Model, info.ModelKind, nil, "", nil, o.start.StartedAt, time.Time{}, nil, nil)
	}
}

func (o *recorderObserver) OnCallEnd(ctx context.Context, info Info) {
	if !o.hasBeg { // tolerate missing start
		o.start = Info{}
	}
	// merge fields
	if len(info.RequestJSON) == 0 {
		info.RequestJSON = o.start.RequestJSON
	}
	if info.StartedAt.IsZero() {
		info.StartedAt = o.start.StartedAt
	}
	// attach to message/turn from context
	msgID := memory.MessageIDFromContext(ctx)
	turnID := memory.TurnIDFromContext(ctx)
	if msgID == "" || o.r == nil || !o.r.Enabled() {
		return
	}
	o.r.RecordModelCall(ctx, msgID, turnID, info.Provider, info.Model, info.ModelKind, info.Usage, info.FinishReason, info.Cost, info.StartedAt, info.CompletedAt, info.RequestJSON, info.ResponseJSON)
}

// WithRecorderObserver injects a recorder-backed Observer into context.
func WithRecorderObserver(ctx context.Context, r rec.Recorder) context.Context {
	if r == nil || !r.Enabled() {
		return ctx
	}
	return WithObserver(ctx, &recorderObserver{r: r})
}
