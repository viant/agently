package modelcall

import (
	"context"

	write "github.com/viant/agently/pkg/agently/modelcall"
)

// API defines a backend-agnostic contract for model call DAO.
type API interface {
	Patch(ctx context.Context, calls ...*write.ModelCall) (*write.Output, error)
}
