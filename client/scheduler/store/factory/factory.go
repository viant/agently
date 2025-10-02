package factory

import (
	"context"

	schcli "github.com/viant/agently/client/scheduler/store"
	convinternal "github.com/viant/agently/internal/service/conversation"
	internal "github.com/viant/agently/internal/service/scheduler/store"
	"github.com/viant/datly"
)

// New constructs the scheduler store API using the provided datly service.
func New(ctx context.Context, dao *datly.Service) (schcli.Client, error) {
	return internal.New(ctx, dao)
}

// NewFromEnv constructs a datly service from environment and returns the scheduler store API.
func NewFromEnv(ctx context.Context) (schcli.Client, error) {
	dao, err := convinternal.NewDatlyServiceFromEnv(ctx)
	if err != nil {
		return nil, err
	}
	return internal.New(ctx, dao)
}
