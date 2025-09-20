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
	// defaults for new inserts: attempt=1
	for _, tc := range i.ToolCalls {
		if tc == nil {
			continue
		}
		if tc.Has == nil {
			tc.Has = &ToolCallHas{}
		}
		if _, ok := i.CurByID[tc.MessageID]; !ok {
			if !tc.Has.Attempt {
				tc.Attempt = 1
				tc.Has.Attempt = true
			}
		}
	}
	return nil
}

func (i *Input) indexSlice() {
	i.CurByID = map[string]*ToolCall{}
	for _, it := range i.Cur {
		if it != nil {
			i.CurByID[it.MessageID] = it
		}
	}
}
