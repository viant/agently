package usage

import (
	"context"

	"github.com/viant/agently/internal/dao/usage/read"
	"github.com/viant/agently/internal/dao/usage/write"
)

// API defines a backend-agnostic contract for usage DAO.
type API interface {
	List(ctx context.Context, in read.Input) ([]*read.UsageView, error)
	Patch(ctx context.Context, usages ...*write.Usage) (*write.Output, error)
}
