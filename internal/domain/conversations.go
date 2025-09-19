package domain

import (
	"context"

	convread "github.com/viant/agently/internal/dao/conversation/read"
	convwrite "github.com/viant/agently/pkg/agently/conversation/write"
)

// Conversations exposes conversation-oriented operations using DAO read/write models.
// Query methods return DAO read views; modification uses DAO write models.
type Conversations interface {
	// Patch upserts conversations using DAO write model, returning DAO output.
	Patch(ctx context.Context, conversations ...*convwrite.Conversation) (*convwrite.Output, error)

	// Get returns a DAO conversation view by id (nil,nil when not found).
	Get(ctx context.Context, id string) (*convread.ConversationView, error)

	// List returns conversations using DAO read input options.
	List(ctx context.Context, opts ...convread.ConversationInputOption) ([]*convread.ConversationView, error)
}
