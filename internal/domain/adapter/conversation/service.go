package conversation

import (
	"context"

	convdao "github.com/viant/agently/internal/dao/conversation"
	convread "github.com/viant/agently/internal/dao/conversation/read"
	convwrite "github.com/viant/agently/internal/dao/conversation/write"
	usagedao "github.com/viant/agently/internal/dao/usage"
	usagew "github.com/viant/agently/internal/dao/usage/write"
	d "github.com/viant/agently/internal/domain"
)

// Service adapts conversation DAO APIs to the domain.Conversations interface.
type Service struct {
	conv  convdao.API
	usage usagedao.API
}

func New(conv convdao.API, usage usagedao.API) *Service { return &Service{conv: conv, usage: usage} }

var _ d.Conversations = (*Service)(nil)

// Patch upserts conversations using DAO write model.
func (s *Service) Patch(ctx context.Context, conversations ...*convwrite.Conversation) (*convwrite.Output, error) {
	if s == nil || s.conv == nil {
		return &convwrite.Output{}, nil
	}
	return s.conv.PatchConversations(ctx, conversations...)
}

// Get returns a single conversation view by id.
func (s *Service) Get(ctx context.Context, id string) (*convread.ConversationView, error) {
	if s == nil || s.conv == nil {
		return nil, nil
	}
	return s.conv.GetConversation(ctx, id)
}

// List queries conversations using input options.
func (s *Service) List(ctx context.Context, opts ...convread.ConversationInputOption) ([]*convread.ConversationView, error) {
	if s == nil || s.conv == nil {
		return []*convread.ConversationView{}, nil
	}
	return s.conv.GetConversations(ctx, opts...)
}

// UpdateUsageTotals writes usage counters via usage write component.
func (s *Service) UpdateUsageTotals(ctx context.Context, id string, totals d.UsageTotals) error {
	if s == nil || s.usage == nil {
		return nil
	}
	w := &usagew.Usage{}
	w.SetConversationID(id)
	w.SetUsageInputTokens(totals.InputTokens)
	w.SetUsageOutputTokens(totals.OutputTokens)
	w.SetUsageEmbeddingTokens(totals.EmbeddingTokens)
	_, err := s.usage.Patch(ctx, w)
	return err
}
