package sql

import (
	"context"
	"fmt"

	"github.com/viant/agently/internal/dao/turn/read"
	"github.com/viant/agently/pkg/agently/turn"
	"github.com/viant/datly"
)

// DefineComponent registers read and write components for turn.
func DefineComponent(ctx context.Context, srv *datly.Service) error {
	if err := read.DefineComponent(ctx, srv); err != nil {
		return fmt.Errorf("failed to add turn read: %w", err)
	}
	if _, err := turn.DefineComponent(ctx, srv); err != nil {
		return fmt.Errorf("failed to add turn write: %w", err)
	}
	return nil
}
