package agent

import (
	"context"
)

type warningsKey struct{}

// withWarnings returns a derived context carrying a warnings slice pointer.
func withWarnings(ctx context.Context) (context.Context, *[]string) {
	arr := []string{}
	return context.WithValue(ctx, warningsKey{}, &arr), &arr
}

// appendWarning appends a warning message to the slice carried in ctx, if any.
func appendWarning(ctx context.Context, msg string) {
	if msg == "" {
		return
	}
	if v := ctx.Value(warningsKey{}); v != nil {
		if p, ok := v.(*[]string); ok && p != nil {
			*p = append(*p, msg)
		}
	}
}

// warningsFrom returns a copy of the warnings slice carried in ctx, if any.
func warningsFrom(ctx context.Context) []string {
	if v := ctx.Value(warningsKey{}); v != nil {
		if p, ok := v.(*[]string); ok && p != nil {
			out := make([]string, len(*p))
			copy(out, *p)
			return out
		}
	}
	return nil
}
