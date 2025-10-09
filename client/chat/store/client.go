package store

import (
	"context"

	chat "github.com/viant/agently/client/chat"
	agconv "github.com/viant/agently/pkg/agently/conversation"
)

// Client defines CRUD-style operations for chat conversations, messages,
// payloads, model/tool calls, and turns. It mirrors conversation.Client but
// resides under chat/store for coherence with other store clients.
type Client interface {
	// Reads: list returns Output; single returns View
	GetConversation(ctx context.Context, id string, options ...chat.Option) (*agconv.ConversationView, error)
	GetConversations(ctx context.Context) (*agconv.ConversationOutput, error)
	PatchConversations(ctx context.Context, conversations *chat.MutableConversation) error

	GetPayload(ctx context.Context, id string) (*chat.Payload, error)
	PatchPayload(ctx context.Context, payload *chat.MutablePayload) error

	GetMessage(ctx context.Context, id string) (*chat.Message, error)
	GetMessageByElicitation(ctx context.Context, conversationID, elicitationID string) (*chat.Message, error)
	PatchMessage(ctx context.Context, message *chat.MutableMessage) error

	PatchModelCall(ctx context.Context, modelCall *chat.MutableModelCall) error
	PatchToolCall(ctx context.Context, toolCall *chat.MutableToolCall) error
	PatchTurn(ctx context.Context, turn *chat.MutableTurn) error

	DeleteConversation(ctx context.Context, id string) error
}
