package usage

import (
	"context"
	"fmt"

	dao "github.com/viant/agently/internal/dao/usage"
	read "github.com/viant/agently/internal/dao/usage/read"
	write "github.com/viant/agently/internal/dao/usage/write"
	d "github.com/viant/agently/internal/domain"
)

// Service adapts usage DAO to domain.Usage.
type Service struct{ dao dao.API }

func New(dao dao.API) *Service { return &Service{dao: dao} }

var _ d.Usage = (*Service)(nil)

func (s *Service) List(ctx context.Context, in read.Input) ([]*read.UsageView, error) {
	if s == nil || s.dao == nil {
		return nil, fmt.Errorf("usage service is not configured")
	}
	return s.dao.List(ctx, in)
}

func (s *Service) Patch(ctx context.Context, usages ...*write.Usage) (*write.Output, error) {
	if s == nil || s.dao == nil {
		return nil, fmt.Errorf("usage service is not configured")
	}
	return s.dao.Patch(ctx, usages...)
}
