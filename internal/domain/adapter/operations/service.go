package operations

import (
	"context"

	daofactory "github.com/viant/agently/internal/dao/factory"
	tcwrite "github.com/viant/agently/internal/dao/toolcall/write"
	d "github.com/viant/agently/internal/domain"
	mcwrite "github.com/viant/agently/pkg/agently/modelcall"
)

// Service implements domain.Operations using DAO write components.
type Service struct {
	api *daofactory.API
}

var _ d.Operations = (*Service)(nil)

func New(api *daofactory.API) *Service { return &Service{api: api} }

func (s *Service) RecordModelCall(ctx context.Context, call *mcwrite.ModelCall) error {
	if s == nil || s.api == nil || s.api.ModelCall == nil || call == nil {
		return nil
	}
	_, err := s.api.ModelCall.Patch(ctx, call)
	return err
}

func (s *Service) RecordToolCall(ctx context.Context, call *tcwrite.ToolCall) error {
	if s == nil || s.api == nil || s.api.ToolCall == nil || call == nil {
		return nil
	}
	_, err := s.api.ToolCall.Patch(ctx, call)
	return err
}
