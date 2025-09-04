package turn

import (
	"context"
	"fmt"

	turndao "github.com/viant/agently/internal/dao/turn"
	read "github.com/viant/agently/internal/dao/turn/read"
	write "github.com/viant/agently/internal/dao/turn/write"
	d "github.com/viant/agently/internal/domain"
)

// Service adapts Turn DAO to domain.Turns.
type Service struct{ dao turndao.API }

func New(dao turndao.API) *Service { return &Service{dao: dao} }

var _ d.Turns = (*Service)(nil)

func (s *Service) Start(ctx context.Context, t *write.Turn) (string, error) {
	if s == nil || s.dao == nil {
		return "", fmt.Errorf("turn service is not configured")
	}
	if t == nil {
		return "", fmt.Errorf("nil turn")
	}
	_, err := s.dao.Patch(ctx, t)
	if err != nil {
		return "", err
	}
	return t.Id, nil
}

func (s *Service) Update(ctx context.Context, t *write.Turn) error {
	if s == nil || s.dao == nil {
		return fmt.Errorf("turn service is not configured")
	}
	if t == nil {
		return fmt.Errorf("nil turn")
	}
	_, err := s.dao.Patch(ctx, t)
	return err
}

func (s *Service) List(ctx context.Context, opts ...read.InputOption) ([]*read.TurnView, error) {
	if s == nil || s.dao == nil {
		return nil, fmt.Errorf("turn service is not configured")
	}
	return s.dao.List(ctx, opts...)
}
