package write

import (
	"context"
	"time"

	"github.com/viant/xdatly/handler"
)

func (i *Input) Init(ctx context.Context, sess handler.Session, out *Output) error {
	if err := sess.Stater().Bind(ctx, i); err != nil {
		return err
	}
	if i.Session == nil {
		return nil
	}
	if i.Session.Has == nil {
		i.Session.Has = &SessionHas{}
	}
	now := time.Now().UTC()
	if i.CurSession == nil {
		i.Session.SetCreatedAt(now)
		return nil
	}
	i.Session.Has.Id = true
	i.Session.Has.UserID = true
	i.Session.Has.Provider = true
	i.Session.Has.ExpiresAt = true
	if i.Session.UpdatedAt == nil {
		i.Session.SetUpdatedAt(now)
	}
	return nil
}
