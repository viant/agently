package invoker

import "context"

type keyType struct{}

var ctxKey = keyType{}

// Invoker defines a conversation-scoped tool invoker.
type Invoker interface {
	Invoke(ctx context.Context, service, method string, args map[string]interface{}) (interface{}, error)
}

// With stores an Invoker in context for downstream hooks to use.
func With(ctx context.Context, inv Invoker) context.Context {
	return context.WithValue(ctx, ctxKey, inv)
}

// From extracts an Invoker from context. Returns nil when absent.
func From(ctx context.Context) Invoker {
	if v := ctx.Value(ctxKey); v != nil {
		if inv, ok := v.(Invoker); ok {
			return inv
		}
	}
	return nil
}
