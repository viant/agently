package domain

import (
	"context"
	read "github.com/viant/agently/internal/dao/payload/read"
	write "github.com/viant/agently/pkg/agently/payload"
)

// Payloads exposes payload operations using DAO read/write models.
type Payloads interface {
	// Patch upserts payloads using DAO write model.
	Patch(ctx context.Context, payloads ...*write.Payload) (*write.Output, error)

	// Get returns a DAO payload view by id (nil,nil if not found).
	Get(ctx context.Context, id string) (*read.PayloadView, error)

	// List returns DAO payload views matching input options.
	List(ctx context.Context, opts ...read.InputOption) ([]*read.PayloadView, error)
}
