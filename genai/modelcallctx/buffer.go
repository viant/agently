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
	Payload      []byte
	LLMResponse  *llm.GenerateResponse
	StreamText   string
	Usage        *llm.Usage
	StartedAt    time.Time
	CompletedAt  time.Time
	Err          string
	ErrorCode    string
	FinishReason string
	Cost         *float64
}

// ObserverFromContext returns the explicitly injected Observer stored in ctx (or nil).
// Callers must inject an Observer (for example, WithRecorderObserver) before
// invoking LLM providers so that OnCallStart/OnCallEnd are delivered.
func ObserverFromContext(ctx context.Context) Observer {
	if v := ctx.Value(observerKey); v != nil {
		if ob, ok := v.(Observer); ok {
			return ob
		}
	}
	return nil
}

type bufferKeyT struct{}

var bufferKey = bufferKeyT{}

type observerKeyT struct{}

var observerKey = observerKeyT{}

type Buffer struct {
	items     []Info
	lastStart *Info
}

// Observer exposes OnCallStart/OnCallEnd used by providers.
type Observer interface {
	OnCallStart(ctx context.Context, info Info) (context.Context, error)
	OnCallEnd(ctx context.Context, info Info) error
	// OnStreamDelta delivers raw streamed chunks (provider-specific encoding).
	// Implementations may aggregate plain text or persist progressive payloads.
	// Returns error when persistence fails; callers may choose to abort stream.
	OnStreamDelta(ctx context.Context, data []byte) error
}

// WithObserver stores a concrete Observer in context so providers can call it directly.
func WithObserver(ctx context.Context, ob Observer) context.Context {
	return context.WithValue(ctx, observerKey, ob)
}
