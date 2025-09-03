package modelcallctx

import (
	"context"
	"time"

	"github.com/viant/agently/genai/llm"
)

// Info carries a single model-call snapshot.
type Info struct {
	Provider     string
	Model        string
	ModelKind    string
	RequestJSON  []byte
	ResponseJSON []byte
	Usage        *llm.Usage
	StartedAt    time.Time
	CompletedAt  time.Time
	Err          string
	FinishReason string
	Cost         *float64
}

type bufferKeyT struct{}

var bufferKey = bufferKeyT{}

type Buffer struct {
	items     []Info
	lastStart *Info
}

func WithBuffer(ctx context.Context) (context.Context, *Buffer) {
	if ctx == nil {
		ctx = context.Background()
	}
	b := &Buffer{}
	return context.WithValue(ctx, bufferKey, b), b
}

func FromContext(ctx context.Context) *Buffer {
	if ctx == nil {
		return nil
	}
	v := ctx.Value(bufferKey)
	if v == nil {
		return nil
	}
	if b, ok := v.(*Buffer); ok {
		return b
	}
	return nil
}

// Observer exposes OnCallStart/OnCallEnd used by providers.
type Observer interface {
	OnCallStart(ctx context.Context, info Info)
	OnCallEnd(ctx context.Context, info Info)
}

type bufferObserver struct{}

func (bufferObserver) OnCallStart(ctx context.Context, info Info) {
	if b := FromContext(ctx); b != nil {
		// Remember start snapshot so we can merge into completion later.
		// Also keep it in the sequence for debugging if needed.
		b.lastStart = &info
		b.items = append(b.items, info)
	}
}

func (bufferObserver) OnCallEnd(ctx context.Context, info Info) {
	if b := FromContext(ctx); b != nil {
		// Merge request/start fields when available so consumers
		// receive both request and response in a single snapshot.
		if b.lastStart != nil {
			if len(info.RequestJSON) == 0 && len(b.lastStart.RequestJSON) > 0 {
				info.RequestJSON = b.lastStart.RequestJSON
			}
			if info.StartedAt.IsZero() && !b.lastStart.StartedAt.IsZero() {
				info.StartedAt = b.lastStart.StartedAt
			}
			// clear lastStart to avoid leaking stale starts across calls
			b.lastStart = nil
		}
		// append as a new item; phases gather last completed
		b.items = append(b.items, info)
	}
}

// ObserverFromContext returns an Observer that writes into the buffer in ctx.
func ObserverFromContext(ctx context.Context) Observer {
	if FromContext(ctx) == nil {
		return nil
	}
	return bufferObserver{}
}

// PopLast returns the most recent completed call info.
func PopLast(ctx context.Context) (Info, bool) {
	b := FromContext(ctx)
	if b == nil || len(b.items) == 0 {
		return Info{}, false
	}
	// find last item with CompletedAt set or just the last
	for i := len(b.items) - 1; i >= 0; i-- {
		it := b.items[i]
		if !it.CompletedAt.IsZero() {
			// shrink slice
			b.items = append(b.items[:i], b.items[i+1:]...)
			return it, true
		}
	}
	// fallback last
	it := b.items[len(b.items)-1]
	b.items = b.items[:len(b.items)-1]
	return it, true
}
