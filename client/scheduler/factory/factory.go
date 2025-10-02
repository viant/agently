package factory

import (
	"context"

	schcli "github.com/viant/agently/client/scheduler"
	internal "github.com/viant/agently/internal/service/scheduler"
)

// NewFromEnv constructs a scheduler service client using environment-backed
// datly configuration and default chat wiring. It simplifies consumer wiring.
func NewFromEnv(ctx context.Context) (schcli.Client, error) {
	return internal.NewFromEnv(ctx)
}
