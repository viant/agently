package modelcall

import (
	"context"
	read "github.com/viant/agently/internal/dao/modelcall/read"
	write "github.com/viant/agently/internal/dao/modelcall/write"
)

// API defines a backend-agnostic contract for model call DAO.
type API interface {
	List(ctx context.Context, opts ...read.InputOption) ([]*read.ModelCallView, error)
	Patch(ctx context.Context, calls ...*write.ModelCall) (*write.Output, error)
}
