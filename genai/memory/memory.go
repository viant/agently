package memory

import (
	"context"
	"sync"
)

// ModelMessageIDKey carries the message id to which the current model call should attach.
type modelMessageIDKey string

var ModelMessageIDKey = modelMessageIDKey("modelMessageID")

func ModelMessageIDFromContext(ctx context.Context) string {
	value := ctx.Value(ModelMessageIDKey)
	if value == nil {
		return ""
	}
	return value.(string)
}

// TurnMeta captures minimal per-turn context for downstream persistence.
// Prefer passing a single TurnMeta instead of scattering separate keys.
type TurnMeta struct {
	TurnID          string
	Assistant       string
	ConversationID  string
	ParentMessageID string // last user message id (or tool message when parenting final)
}

type turnMetaKeyT string

var turnMetaKey = turnMetaKeyT("turnMeta")

// turnTrace holds a per-turn provider trace/anchor id (e.g., OpenAI response.id)
var turnTrace sync.Map // key: turnID string -> value: traceID string

// SetTurnTrace stores a provider trace/anchor id for the given turn id.
func SetTurnTrace(turnID, traceID string) {
	if turnID == "" || traceID == "" {
		return
	}
	turnTrace.Store(turnID, traceID)
}

// TurnTrace returns a previously stored provider trace/anchor id for this turn.
func TurnTrace(turnID string) string {
	if turnID == "" {
		return ""
	}
	if v, ok := turnTrace.Load(turnID); ok {
		if s, ok2 := v.(string); ok2 {
			return s
		}
	}
	return ""
}

// Deprecated in-memory tool_call anchors have been removed.

// WithTurnMeta stores TurnMeta on the context and also seeds individual keys
// for backward compatibility with existing readers.
func WithTurnMeta(ctx context.Context, meta TurnMeta) context.Context {

	if meta.ConversationID != "" {
		ctx = context.WithValue(ctx, ConversationIDKey, meta.ConversationID)
	}
	return context.WithValue(ctx, turnMetaKey, meta)
}

// TurnMetaFromContext returns a stored TurnMeta when present.
func TurnMetaFromContext(ctx context.Context) (TurnMeta, bool) {
	if ctx == nil {
		return TurnMeta{}, false
	}
	if v := ctx.Value(turnMetaKey); v != nil {
		if m, ok := v.(TurnMeta); ok {
			return m, true
		}
	}
	return TurnMeta{}, false
}

// EmbedFunc defines a function that creates embeddings for given texts.
// It should return one embedding per input text.
type EmbedFunc func(ctx context.Context, texts []string) ([][]float32, error)
