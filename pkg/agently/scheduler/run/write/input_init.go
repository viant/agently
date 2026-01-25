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
	now := time.Now().UTC()
	for _, r := range i.Runs {
		if r == nil {
			continue
		}
		if _, ok := i.CurRunById[r.Id]; !ok {
			if r.CreatedAt == nil {
				r.SetCreatedAt(now)
			}
			continue
		}
		if r.UpdatedAt == nil {
			r.SetUpdatedAt(now)
		}
	}
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
