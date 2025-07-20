package conversation

// This file introduces helper utilities to embed and retrieve the current
// conversation ID inside a context.Context. The identifier travels with every
// Fluxor dispatch, MCP call, or HTTP handler so that downstream services can
// isolate per-conversation resources without modifying action payloads.

import "context"

// ctxKeyType is an unexported, unique type used as the map key in
// context.WithValue. A distinct type avoids collisions with keys defined by
// external packages.
type ctxKeyType struct{}

var ctxKey = ctxKeyType{}

// WithID returns a new context that carries the supplied conversation ID.
// If the incoming context is not a Fluxor execution.Context it is wrapped in
// one so that reducers/effects can still access Fluxor helpers.
func WithID(ctx context.Context, id string) context.Context {
	// Do not force wrap into fluxor execution.Context; upstream services
	// create the enriched context with proper event/publisher initialisation.
	return context.WithValue(ctx, ctxKey, id)
}

// EnsureID is a backward-compatible alias that mirrors the previous helper
// name used across the codebase. It behaves identical to WithID.
func EnsureID(ctx context.Context, id string) context.Context {
	return WithID(ctx, id)
}

// ID extracts the conversation ID from ctx. When ctx does not carry an ID the
// empty string is returned.
func ID(ctx context.Context) string {
	v, _ := ctx.Value(ctxKey).(string)
	return v
}
