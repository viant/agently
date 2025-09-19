package modelcall

import (
	"context"

	"github.com/viant/xdatly/handler"
)

func (i *Input) Init(ctx context.Context, sess handler.Session, _ *Output) error {
	if err := sess.Stater().Bind(ctx, i); err != nil {
		return err
	}
	i.indexSlice()
	// apply defaults for NOT NULL columns when inserting new records
	for _, mc := range i.ModelCalls {
		if mc == nil {
			continue
		}
		if _, exists := i.CurByID[mc.MessageID]; !exists {
			if mc.CacheHit == nil {
				mc.SetCacheHit(0)
			}
		}
	}
	return nil
}

func (i *Input) indexSlice() {
	i.CurByID = map[string]*ModelCall{}
	for _, it := range i.Cur {
		if it != nil {
			i.CurByID[it.MessageID] = it
		}
	}
}
