package turn

import (
	"context"

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
	if s == nil || s.dao == nil || t == nil {
		if t != nil {
			return t.Id, nil
		}
		return "", nil
	}
	_, err := s.dao.Patch(ctx, t)
	if err != nil {
		return "", err
	}
	return t.Id, nil
}

func (s *Service) Update(ctx context.Context, t *write.Turn) error {
	if s == nil || s.dao == nil || t == nil {
		return nil
	}
	_, err := s.dao.Patch(ctx, t)
	return err
}

func (s *Service) List(ctx context.Context, opts ...read.InputOption) ([]*read.TurnView, error) {
	if s == nil || s.dao == nil {
		return []*read.TurnView{}, nil
	}
	return s.dao.List(ctx, opts...)
}
