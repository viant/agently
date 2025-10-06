package write

import (
	"context"
	"github.com/viant/xdatly/handler"
)

func (i *Input) Init(ctx context.Context, sess handler.Session, _ *Output) error {
	if err := sess.Stater().Bind(ctx, i); err != nil {
		return err
	}
	i.indexSlice()
	return nil
}

func (i *Input) indexSlice() {
	i.CurUserById = map[string]*User{}
	for _, m := range i.CurUser {
		if m != nil {
			i.CurUserById[m.Id] = m
		}
	}
}
