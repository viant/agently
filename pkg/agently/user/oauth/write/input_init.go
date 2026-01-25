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
	if i.Token == nil {
		return nil
	}
	if i.Token.Has == nil {
		i.Token.Has = &TokenHas{}
	}
	now := time.Now().UTC()
	if i.CurToken == nil {
		i.Token.SetCreatedAt(now)
		return nil
	}
	i.Token.Has.UserID = true
	i.Token.Has.Provider = true
	i.Token.Has.EncToken = true
	if i.Token.UpdatedAt == nil {
		i.Token.SetUpdatedAt(now)
	}
	return nil
}
