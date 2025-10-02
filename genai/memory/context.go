package memory

import (
	"context"

	"github.com/google/uuid"
)

// ConversationIDKey is used to propagate the current conversation identifier
// via context so that downstream services (e.g. tool-execution tracing) can
// associate side-effects with the correct conversation without changing every
// function signature.

var ConversationIDKey = "conversationID"

// ConversationIDFromContext return conversation id from context
func ConversationIDFromContext(ctx context.Context) string {
	value := ctx.Value(ConversationIDKey)
	if value == nil {
		return ""
	}
	return value.(string)
}

// WithConversationID create context with conversation id
func WithConversationID(ctx context.Context, conversationID string) context.Context {
	if conversationID == "" {
		conversationID = uuid.New().String()
	}
	prev := ctx.Value(ConversationIDKey)
	if prev != nil && prev == conversationID {
		return ctx
	}
	return context.WithValue(ctx, ConversationIDKey, conversationID)
}
