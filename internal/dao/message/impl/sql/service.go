package sql

import (
	"context"

	write "github.com/viant/agently/pkg/agently/message"
	"github.com/viant/datly"
	"github.com/viant/datly/repository/contract"
)

type Service struct{ dao *datly.Service }

func New(ctx context.Context, dao *datly.Service) *Service { return &Service{dao: dao} }

// Register components (to be invoked by parent module).
func Register(ctx context.Context, dao *datly.Service) error {
	if _, err := write.DefineComponent(ctx, dao); err != nil {
		return err
	}
	return nil
}

// Patch upserts messages via write component
func (s *Service) Patch(ctx context.Context, messages ...*write.Message) (*write.Output, error) {
	in := &write.Input{Messages: messages}
	out := &write.Output{}
	_, err := s.dao.Operate(ctx,
		datly.WithPath(contract.NewPath("PATCH", write.PathURI)),
		datly.WithInput(in),
		datly.WithOutput(out),
	)
	if err != nil {
		return nil, err
	}
	return out, nil
}
