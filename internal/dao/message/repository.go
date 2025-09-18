package message

import (
	"context"

	"github.com/viant/agently/pkg/agently/message"
)

// API defines a backend-agnostic contract for message DAO.
type API interface {
	Patch(ctx context.Context, messages ...*message.Message) (*message.Output, error)
}
