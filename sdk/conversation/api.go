package conversation

import (
	"context"

	"github.com/viant/agently/pkg/agently/conversation"
)

type API interface {
	Get(ctx context.Context, id string, options ...Option) (*Conversation, error)
}

type Input conversation.ConversationInput
