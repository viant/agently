package conversation

import (
	"context"
)

type Client interface {
	GetConversation(ctx context.Context, id string, options ...Option) (*Conversation, error)
	GetConversations(ctx context.Context, input *Input) ([]*Conversation, error)
	PatchConversations(ctx context.Context, conversations *MutableConversation) error
	GetPayload(ctx context.Context, id string) (*Payload, error)
	PatchPayload(ctx context.Context, payload *MutablePayload) error
	PatchMessage(ctx context.Context, message *MutableMessage) error
	GetMessage(ctx context.Context, id string) (*Message, error)
	GetMessageByElicitation(ctx context.Context, conversationID, elicitationID string) (*Message, error)
	PatchModelCall(ctx context.Context, modelCall *MutableModelCall) error
	PatchToolCall(ctx context.Context, toolCall *MutableToolCall) error
	PatchTurn(ctx context.Context, turn *MutableTurn) error
	DeleteConversation(ctx context.Context, id string) error
	// DeleteMessage removes a message from the conversation and internal indexes.
	DeleteMessage(ctx context.Context, conversationID, messageID string) error
}
