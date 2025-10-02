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
	i.CurRunById = map[string]*Run{}
	for _, m := range i.CurRun {
		if m != nil {
			i.CurRunById[m.Id] = m
		}
	}
}
