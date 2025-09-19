package conversation

import (
	"context"

	read2 "github.com/viant/agently/internal/dao/conversation/read"
	"github.com/viant/agently/pkg/agently/conversation"
	"github.com/viant/agently/pkg/agently/conversation/write"
)

// API defines a backend-agnostic contract for conversation DAO.
// Implementations: SQL (existing) and memory (new).
type API interface {
	GetConversations(ctx context.Context, opts ...read2.ConversationInputOption) ([]*read2.ConversationView, error)
	GetConversation(ctx context.Context, convID string) (*read2.ConversationView, error)
	PatchConversations(ctx context.Context, conversations ...*write.Conversation) (*write.Output, error)
}

// API defines a backend-agnostic contract for conversation DAO.
// Implementations: SQL (existing) and memory (new).
type APIV2 interface {
	GetConversationRich(ctx context.Context, convID string) (*conversation.ConversationView, error)
}
