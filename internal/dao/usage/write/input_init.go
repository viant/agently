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
	i.CurByID = map[string]*Usage{}
	for _, it := range i.Cur {
		if it != nil {
			i.CurByID[it.Id] = it
		}
	}
}
