package sql

import (
	"context"
	"net/http"
	"strings"

	"github.com/viant/agently/internal/dao/usage"
	"github.com/viant/agently/internal/dao/usage/read"
	write2 "github.com/viant/agently/internal/dao/usage/write"
	"github.com/viant/datly"
	"github.com/viant/datly/repository/contract"
)

type Service struct{ dao *datly.Service }

func New(ctx context.Context, dao *datly.Service) *Service { return &Service{dao: dao} }

func Register(ctx context.Context, dao *datly.Service) error { return usage.Register(ctx, dao) }

// List wraps datly operation for read usage
func (s *Service) List(ctx context.Context, in read.Input) ([]*read.UsageView, error) {
	out := &read.Output{}
	if in.Has != nil && in.Has.ConversationID && in.ConversationID != "" {
		uri := strings.ReplaceAll(read.PathByConversation, "{conversationId}", in.ConversationID)
		_, err := s.dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(uri), datly.WithInput(&in))
		if err != nil {
			return nil, err
		}
		return out.Data, nil
	}
	_, err := s.dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(read.PathBase), datly.WithInput(&in))
	if err != nil {
		return nil, err
	}
	return out.Data, nil
}

// Patch updates conversation usage tokens totals
func (s *Service) Patch(ctx context.Context, usages ...*write2.Usage) (*write2.Output, error) {
	in := &write2.Input{Usages: usages}
	out := &write2.Output{}
	_, err := s.dao.Operate(ctx,
		datly.WithPath(contract.NewPath(http.MethodPatch, write2.PathURI)),
		datly.WithInput(in),
		datly.WithOutput(out),
	)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Re-exports for ergonomics
type Input = read.Input
type UsageView = read.UsageView
