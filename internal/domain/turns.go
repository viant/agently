package domain

import (
	"context"
	read "github.com/viant/agently/internal/dao/turn/read"
	write "github.com/viant/agently/internal/dao/turn/write"
)

// Turns exposes turn lifecycle operations.
type Turns interface {
	// Start registers a new turn via DAO write model and returns its id.
	Start(ctx context.Context, t *write.Turn) (string, error)

	// Update applies changes to a turn via DAO write model.
	Update(ctx context.Context, t *write.Turn) error

	// List returns DAO read TurnView rows using input options.
	List(ctx context.Context, opts ...read.InputOption) ([]*read.TurnView, error)
}
