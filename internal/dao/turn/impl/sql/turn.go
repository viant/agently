package sql

import (
	"context"
	"fmt"

	"github.com/viant/agently/internal/dao/turn/read"
	"github.com/viant/agently/internal/dao/turn/write"
	"github.com/viant/datly"
)

// DefineComponent registers read and write components for turns.
func DefineComponent(ctx context.Context, srv *datly.Service) error {
	if err := read.DefineComponent(ctx, srv); err != nil {
		return fmt.Errorf("failed to add turn read: %w", err)
	}
	if _, err := write.DefineComponent(ctx, srv); err != nil {
		return fmt.Errorf("failed to add turn write: %w", err)
	}
	return nil
}
