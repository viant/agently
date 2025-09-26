package convctx

// Minimal, cycle-free helpers to embed and retrieve a conversation ID in
// context. Kept separate from genai/conversation to avoid import cycles with
// tool/agent packages.

import "context"

type ctxKeyType struct{}

var ctxKey = ctxKeyType{}

// WithID returns a new context that carries the supplied conversation ID.
func WithID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey, id)
}

// EnsureID is an alias of WithID for backward compatibility.
func EnsureID(ctx context.Context, id string) context.Context { return WithID(ctx, id) }

// ID extracts the conversation ID from ctx; empty string when absent.
func ID(ctx context.Context) string {
	v, _ := ctx.Value(ctxKey).(string)
	return v
}
