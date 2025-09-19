package factory

import (
	"context"

	convcli "github.com/viant/agently/client/conversation"
	internal "github.com/viant/agently/internal/service/conversation"
	"github.com/viant/datly"
)

// New constructs the conversation API using the provided datly service.
func New(ctx context.Context, dao *datly.Service) (convcli.Client, error) {
	return internal.New(ctx, dao)
}

// NewFromEnv constructs a datly service from environment and returns the conversation API.
func NewFromEnv(ctx context.Context) (convcli.Client, error) {
	dao, err := internal.NewDatlyServiceFromEnv(ctx)
	if err != nil {
		return nil, err
	}
	return internal.New(ctx, dao)
}
