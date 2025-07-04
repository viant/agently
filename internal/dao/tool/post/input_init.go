package post

import (
	"context"
	"github.com/viant/xdatly/handler"
)

func (i *Input) Init(ctx context.Context, sess handler.Session, output *Output) error {
	if err := sess.Stater().Bind(ctx, i); err != nil {
		return err
	}
	i.indexSlice()
	//TODO: add your custom init logic here
	return nil
}

func (i *Input) indexSlice() {
	i.CurToolById = ToolCallSlice(i.ToolCall).IndexById()
}
