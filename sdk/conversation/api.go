package conversation

import (
	"context"

	"github.com/viant/agently/pkg/agently/conversation"
)

type API interface {
	GetConversation(ctx context.Context, id string, options ...Option) (*Conversation, error)
	GetConversations(ctx context.Context) ([]*Conversation, error)
}

type Input conversation.ConversationInput
