package executil

import (
	"context"
	"os"
	"strings"
	"time"
)

// toolExecContext returns a bounded context for tool execution. It uses AGENTLY_TOOLCALL_TIMEOUT
// environment variable when set (e.g., "45s", "2m"), otherwise defaults to 60s.
func toolExecContext(ctx context.Context) (context.Context, context.CancelFunc) {
	const defaultTimeout = 3 * time.Minute
	if d, ok := toolTimeoutFromContext(ctx); ok && d > 0 {
		return context.WithTimeout(ctx, d)
	}
	if v := strings.TrimSpace(os.Getenv("AGENTLY_TOOLCALL_TIMEOUT")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return context.WithTimeout(ctx, d)
		}
	}
	return context.WithTimeout(ctx, defaultTimeout)
}

// ---------------- context helpers ----------------

type ctxKey int

const (
	keyToolTimeout ctxKey = iota + 1
	keyChainMode
)

// WithToolTimeout attaches a per-tool execution timeout to the context.
func WithToolTimeout(ctx context.Context, d time.Duration) context.Context {
	return context.WithValue(ctx, keyToolTimeout, d)
}

// toolTimeoutFromContext reads a configured tool timeout from context when present.
func toolTimeoutFromContext(ctx context.Context) (time.Duration, bool) {
	if v := ctx.Value(keyToolTimeout); v != nil {
		if d, ok := v.(time.Duration); ok {
			return d, true
		}
	}
	return 0, false
}

// WithChainMode marks tool/message execution context as internal chain execution.
func WithChainMode(ctx context.Context, enabled bool) context.Context {
	return context.WithValue(ctx, keyChainMode, enabled)
}

// IsChainMode reports whether current execution is internal chain execution.
func IsChainMode(ctx context.Context) bool {
	if v := ctx.Value(keyChainMode); v != nil {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}
