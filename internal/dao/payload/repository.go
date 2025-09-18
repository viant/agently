package payload

import (
	"context"

	read "github.com/viant/agently/internal/dao/payload/read"
	write "github.com/viant/agently/pkg/agently/payload"
)

// API defines a backend-agnostic contract for payload DAO.
type API interface {
	List(ctx context.Context, opts ...read.InputOption) ([]*read.PayloadView, error)
	Patch(ctx context.Context, payloads ...*write.Payload) (*write.Output, error)
}
