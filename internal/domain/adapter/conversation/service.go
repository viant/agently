package conversation

import (
	"context"
	"fmt"

	convdao "github.com/viant/agently/internal/dao/conversation"
	convread "github.com/viant/agently/internal/dao/conversation/read"
	d "github.com/viant/agently/internal/domain"
	convwrite "github.com/viant/agently/pkg/agently/conversation/write"
)

// Service adapts conversation DAO APIs to the domain.Conversations interface.
type Service struct {
	conv convdao.API
}

func New(conv convdao.API) *Service { return &Service{conv: conv} }

var _ d.Conversations = (*Service)(nil)

// Patch upserts conversations using DAO write model.
func (s *Service) Patch(ctx context.Context, conversations ...*convwrite.Conversation) (*convwrite.Output, error) {
	if s == nil || s.conv == nil {
		return nil, fmt.Errorf("conversation service is not configured")
	}
	return s.conv.PatchConversations(ctx, conversations...)
}

// Get returns a single conversation view by id.
func (s *Service) Get(ctx context.Context, id string) (*convread.ConversationView, error) {
	if s == nil || s.conv == nil {
		return nil, fmt.Errorf("conversation service is not configured")
	}
	return s.conv.GetConversation(ctx, id)
}

// List queries conversations using input options.
func (s *Service) List(ctx context.Context, opts ...convread.ConversationInputOption) ([]*convread.ConversationView, error) {
	if s == nil || s.conv == nil {
		return nil, fmt.Errorf("conversation service is not configured")
	}
	return s.conv.GetConversations(ctx, opts...)
}
