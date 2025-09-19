package memory

import (
	"context"
)

// ConversationIDKey is used to propagate the current conversation identifier
// via context so that downstream services (e.g. tool-execution tracing) can
// associate side-effects with the correct conversation without changing every
// function signature.
type conversationID string

var ConversationIDKey = conversationID("conversationID")

func ConversationIDFromContext(ctx context.Context) string {
	value := ctx.Value(ConversationIDKey)
	if value == nil {
		return ""
	}
	return value.(string)
}

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
	ConversationID  string
	ParentMessageID string // last user message id (or tool message when parenting final)
}

type turnMetaKeyT string

var turnMetaKey = turnMetaKeyT("turnMeta")

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

// UserInteraction represents a structured prompt created via the MCP
// user-interaction feature.
type UserInteraction struct {
	URL         string `json:"url" yaml:"url"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

type Attachments []Attachment

// Attachment describes a file linked to the message.
type Attachment struct {
	Name string `json:"name,omitempty" yaml:"name"`
	URL  string `json:"url,omitempty"  yaml:"url"`
	Size int64  `json:"size,omitempty" yaml:"size"` // bytes
	// MediaType allows UI to decide how to display or download.
	MediaType string `json:"mediaType,omitempty" yaml:"mediaType,omitempty"`
}

// EmbedFunc defines a function that creates embeddings for given texts.
// It should return one embedding per input text.
type EmbedFunc func(ctx context.Context, texts []string) ([][]float32, error)
