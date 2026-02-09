package memory

import (
	"context"
	"strings"
)

type contextRecoveryModeKey struct{}

const (
	ContextRecoveryCompact      = "compact"
	ContextRecoveryPruneCompact = "pruneCompact"
)

func WithContextRecoveryMode(ctx context.Context, mode string) context.Context {
	m := strings.TrimSpace(mode)
	if m == "" {
		return ctx
	}
	return context.WithValue(ctx, contextRecoveryModeKey{}, m)
}

func ContextRecoveryModeFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	if v, ok := ctx.Value(contextRecoveryModeKey{}).(string); ok {
		return strings.TrimSpace(v), true
	}
	return "", false
}
