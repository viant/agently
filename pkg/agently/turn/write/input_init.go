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
	for _, t := range i.Turns {
		if t == nil {
			continue
		}
		if t.Has == nil {
			t.Has = &TurnHas{}
		}
		if _, ok := i.CurTurnById[t.Id]; !ok {
			t.SetCreatedAt(now)
		}
	}
	return nil
}

func (i *Input) indexSlice() { i.CurTurnById = TurnSlice(i.CurTurn).IndexById() }
