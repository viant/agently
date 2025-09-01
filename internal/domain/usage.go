package domain

import (
	"context"
	read "github.com/viant/agently/internal/dao/usage/read"
	write "github.com/viant/agently/internal/dao/usage/write"
)

// Usage exposes DAO-oriented usage operations.
type Usage interface {
	// List returns aggregated usage views matching the DAO read input.
	List(ctx context.Context, in read.Input) ([]*read.UsageView, error)

	// Patch updates usage totals (e.g. per-conversation counters) via DAO write model.
	Patch(ctx context.Context, usages ...*write.Usage) (*write.Output, error)
}
