package payload

import (
	"context"

	dao "github.com/viant/agently/internal/dao/payload"
	read "github.com/viant/agently/internal/dao/payload/read"
	write "github.com/viant/agently/internal/dao/payload/write"
	d "github.com/viant/agently/internal/domain"
)

// Service adapts payload DAO to domain.Payloads.
type Service struct{ dao dao.API }

func New(dao dao.API) *Service { return &Service{dao: dao} }

var _ d.Payloads = (*Service)(nil)

// Patch implements domain.Payloads.Patch using DAO write.
func (s *Service) Patch(ctx context.Context, payloads ...*write.Payload) (*write.Output, error) {
	if s == nil || s.dao == nil {
		return &write.Output{}, nil
	}
	return s.dao.Patch(ctx, payloads...)
}

// Get returns a DAO read view by ID.
func (s *Service) Get(ctx context.Context, id string) (*read.PayloadView, error) {
	if s == nil || s.dao == nil {
		return nil, nil
	}
	rows, err := s.dao.List(ctx, read.WithID(id))
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	return rows[0], nil
}

// List returns DAO read views using input options.
func (s *Service) List(ctx context.Context, opts ...read.InputOption) ([]*read.PayloadView, error) {
	if s == nil || s.dao == nil {
		return []*read.PayloadView{}, nil
	}
	return s.dao.List(ctx, opts...)
}
