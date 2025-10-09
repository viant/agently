package factory

import (
	"context"

	chstore "github.com/viant/agently/client/chat/store"
	internal "github.com/viant/agently/internal/service/conversation"
	"github.com/viant/datly"
)

// New constructs a chat store client backed by Datly conversation service.
func New(ctx context.Context, dao *datly.Service) (chstore.Client, error) {
	svc, err := internal.New(ctx, dao)
	if err != nil {
		return nil, err
	}
	return internal.NewStoreAdapter(svc), nil
}

// no init hook required; internal chat service references this factory directly
