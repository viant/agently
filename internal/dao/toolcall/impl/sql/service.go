package sql

import (
	"context"
	"net/http"

	read2 "github.com/viant/agently/internal/dao/toolcall/read"
	"github.com/viant/agently/pkg/agently/toolcall"
	"github.com/viant/datly"
	"github.com/viant/datly/repository/contract"
)

type Service struct{ dao *datly.Service }

func New(ctx context.Context, dao *datly.Service) *Service { return &Service{dao: dao} }

func Register(ctx context.Context, dao *datly.Service) error {
	if err := read2.DefineComponent(ctx, dao); err != nil {
		return err
	}
	if _, err := toolcall.DefineComponent(ctx, dao); err != nil {
		return err
	}
	return nil
}

func (s *Service) Patch(ctx context.Context, calls ...*toolcall.ToolCall) (*toolcall.Output, error) {
	in := &toolcall.Input{ToolCalls: calls}
	out := &toolcall.Output{}
	_, err := s.dao.Operate(ctx,
		datly.WithPath(contract.NewPath(http.MethodPatch, toolcall.PathURI)),
		datly.WithInput(in),
		datly.WithOutput(out),
	)
	if err != nil {
		return nil, err
	}
	return out, nil
}
