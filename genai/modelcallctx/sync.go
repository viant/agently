package modelcallctx

import (
	"context"
	"time"
)

type finishKeyT struct{}

var finishKey = finishKeyT{}

// WithFinishBarrier attaches a barrier channel to ctx so that callers can wait
// until a model call has fully finished (including persistence) before
// proceeding with dependent actions (e.g., emitting final assistant message).
func WithFinishBarrier(ctx context.Context) (context.Context, chan struct{}) {
	ch := make(chan struct{})
	return context.WithValue(ctx, finishKey, ch), ch
}

// signalFinish closes the barrier channel when present.
func signalFinish(ctx context.Context) {
	if v := ctx.Value(finishKey); v != nil {
		if ch, ok := v.(chan struct{}); ok {
			select {
			case <-ch:
				// already closed
			default:
				close(ch)
			}
		}
	}
}

// WaitFinish blocks until the barrier is signaled, ctx is done, or timeout elapses.
// When no barrier is present it returns immediately.
func WaitFinish(ctx context.Context, timeout time.Duration) {
	v := ctx.Value(finishKey)
	ch, _ := v.(chan struct{})
	if ch == nil {
		return
	}
	var to <-chan time.Time
	if timeout > 0 {
		to = time.After(timeout)
	}
	select {
	case <-ch:
		return
	case <-ctx.Done():
		return
	case <-to:
		return
	}
}
