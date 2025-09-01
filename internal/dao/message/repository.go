package message

import (
	"context"
	"github.com/viant/agently/internal/dao/message/read"
	"github.com/viant/agently/internal/dao/message/write"
)

// API defines a backend-agnostic contract for message DAO.
type API interface {
	List(ctx context.Context, opts ...read.InputOption) ([]*read.MessageView, error)
	GetTranscript(ctx context.Context, conversationID, turnID string, opts ...read.InputOption) ([]*read.MessageView, error)
	GetConversation(ctx context.Context, conversationID string, opts ...read.InputOption) ([]*read.MessageView, error)
	Patch(ctx context.Context, messages ...*write.Message) (*write.Output, error)
}
