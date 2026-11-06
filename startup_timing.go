package agently

import (
	"context"
	"log"
	"os"
	"sync/atomic"
	"time"

	"github.com/viant/agently-core/protocol/primitive/observe"
)

// startupTiming uses the existing primitive observer and logs only fixed phase
// names, numeric timing/count/byte values, and bounded outcome enums. It
// never logs source names, account, identity, tokens, URI, or payload.
func startupTiming(ctx context.Context, started time.Time) (context.Context, func(string)) {
	if os.Getenv("BACKEND_PRIMITIVE_TRACE") != "1" {
		return ctx, func(string) {}
	}
	var active atomic.Bool
	active.Store(true)
	observer := observe.ObserverFunc(func(e observe.Event) {
		if !active.Load() {
			return
		}
		end := time.Since(started).Seconds() * 1000
		duration := e.Duration.Seconds() * 1000
		log.Printf("primitive startup phase=%s start_ms=%.3f end_ms=%.3f duration_ms=%.3f calls=1 bytes=%d outcome=%s cache=%s", e.Phase, end-duration, end, duration, e.Bytes, e.Outcome, e.Cache)
	})
	finish := func(outcome string) {
		if !active.Swap(false) {
			return
		}
		end := time.Since(started).Seconds() * 1000
		log.Printf("primitive startup phase=startup.total start_ms=0.000 end_ms=%.3f duration_ms=%.3f calls=1 bytes=0 outcome=%s cache=bypass", end, end, outcome)
	}
	return observe.WithObserver(ctx, observer), finish
}
