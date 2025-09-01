package usage

import (
	"context"
	"fmt"

	"github.com/viant/agently/internal/dao/usage/read"
	"github.com/viant/agently/internal/dao/usage/write"
	"github.com/viant/datly"
)

// DefineComponent registers read and write usage components on the Datly service.
func DefineComponent(ctx context.Context, srv *datly.Service) error {
	if err := read.DefineComponent(ctx, srv); err != nil {
		return fmt.Errorf("failed to add usage read: %w", err)
	}
	if _, err := write.DefineComponent(ctx, srv); err != nil {
		return fmt.Errorf("failed to add usage write: %w", err)
	}
	return nil
}

// Register is an alias to DefineComponent for compatibility with existing code paths.
func Register(ctx context.Context, srv *datly.Service) error { return DefineComponent(ctx, srv) }
