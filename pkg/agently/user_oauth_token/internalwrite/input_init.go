package internalwrite

import (
	"context"
	"github.com/viant/xdatly/handler"
)

// Init binds the input from the Datly session state (supports Operate() input).
func (i *Input) Init(ctx context.Context, sess handler.Session, _ *Output) error {
	return sess.Stater().Bind(ctx, i)
}
