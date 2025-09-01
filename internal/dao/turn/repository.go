package turn

import (
	"context"
	read "github.com/viant/agently/internal/dao/turn/read"
	write "github.com/viant/agently/internal/dao/turn/write"
)

// API defines a backend-agnostic contract for turn DAO.
type API interface {
	List(ctx context.Context, opts ...read.InputOption) ([]*read.TurnView, error)
	Patch(ctx context.Context, turns ...*write.Turn) (*write.Output, error)
}
