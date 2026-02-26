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
	for _, f := range i.GeneratedFiles {
		if f == nil {
			continue
		}
		if f.Has == nil {
			f.Has = &GeneratedFileHas{}
		}
		now := time.Now()
		if _, ok := i.CurByID[f.ID]; !ok {
			if !f.Has.Status || f.Status == "" {
				f.Status = "ready"
				f.Has.Status = true
			}
			if !f.Has.CreatedAt {
				f.CreatedAt = &now
				f.Has.CreatedAt = true
			}
		}
		if !f.Has.UpdatedAt {
			f.UpdatedAt = &now
			f.Has.UpdatedAt = true
		}
	}
	return nil
}

func (i *Input) indexSlice() {
	i.CurByID = map[string]*GeneratedFile{}
	for _, it := range i.Cur {
		if it != nil {
			i.CurByID[it.ID] = it
		}
	}
}
