package write

import (
	"context"
	"time"

	"github.com/viant/xdatly/handler"
)

func (i *Input) Init(ctx context.Context, sess handler.Session, _ *Output) error {
	if err := sess.Stater().Bind(ctx, i); err != nil {
		return err
	}
	i.indexSlice()
	now := time.Now()
	for _, m := range i.Messages {
		if m == nil {
			continue
		}
		if m.Has == nil {
			m.Has = &MessageHas{}
		}
		if _, ok := i.CurMessageById[m.Id]; !ok {
			m.SetCreatedAt(now)
		}
	}
	return nil
}

func (i *Input) indexSlice() {
	i.CurMessageById = map[string]*Message{}
	for _, m := range i.CurMessage {
		if m != nil {
			i.CurMessageById[m.Id] = m
		}
	}
}
