package modelcallctx

import (
	"context"

	agconv "github.com/viant/agently/pkg/agently/conversation"
)

type streamPublisherKey struct{}

// StreamEvent describes a streaming-only message envelope for instant updates.
// It is not persisted; consumers should treat it as ephemeral.
type StreamEvent struct {
	ConversationID string
	Message        *agconv.MessageView
	ContentType    string
	Content        interface{}
}

// StreamPublisher publishes ephemeral stream events (e.g., token deltas).
type StreamPublisher interface {
	Publish(ctx context.Context, ev *StreamEvent) error
}

// WithStreamPublisher injects a StreamPublisher into context.
func WithStreamPublisher(ctx context.Context, p StreamPublisher) context.Context {
	if p == nil {
		return ctx
	}
	return context.WithValue(ctx, streamPublisherKey{}, p)
}

// StreamPublisherFromContext returns a StreamPublisher from context.
func StreamPublisherFromContext(ctx context.Context) (StreamPublisher, bool) {
	if ctx == nil {
		return nil, false
	}
	p, ok := ctx.Value(streamPublisherKey{}).(StreamPublisher)
	return p, ok && p != nil
}
