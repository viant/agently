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
	for _, u := range i.Users {
		if u == nil {
			continue
		}
		if _, ok := i.CurUserById[u.Id]; !ok {
			if u.CreatedAt == nil {
				u.SetCreatedAt(now)
			}
			if u.Disabled == nil {
				u.SetDisabled(0)
			}
			continue
		}
		if u.UpdatedAt == nil {
			u.SetUpdatedAt(now)
		}
	}
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
