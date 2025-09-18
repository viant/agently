package payload

import (
	"context"
	"fmt"

	"github.com/viant/agently/internal/dao/payload/read"
	"github.com/viant/agently/pkg/agently/payload"
	"github.com/viant/datly"
)

// DefineComponent registers read and write components for payloads.
func DefineComponent(ctx context.Context, srv *datly.Service) error {
	if err := read.DefineComponent(ctx, srv); err != nil {
		return fmt.Errorf("failed to add payload read: %w", err)
	}
	if _, err := payload.DefineComponent(ctx, srv); err != nil {
		return fmt.Errorf("failed to add payload write: %w", err)
	}
	return nil
}

// Register kept for compatibility with existing code paths
func Register(ctx context.Context, srv *datly.Service) error { return DefineComponent(ctx, srv) }
