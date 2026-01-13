package write

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/viant/xdatly/handler"
	"github.com/viant/xdatly/handler/response"
)

type Handler struct{}

func (h *Handler) Exec(ctx context.Context, sess handler.Session) (interface{}, error) {
	out := &Output{}
	out.Status.Status = "ok"
	if err := h.exec(ctx, sess, out); err != nil {
		var rErr *response.Error
		if errors.As(err, &rErr) {
			return out, err
		}
		out.setError(err)
	}

	if len(out.Violations) > 0 {
		out.setError(fmt.Errorf("failed validation"))
		return out, response.NewError(http.StatusBadRequest, "bad request")
	}
	return out, nil
}

func (h *Handler) exec(ctx context.Context, sess handler.Session, out *Output) error {
	in := &Input{}
	if err := in.Init(ctx, sess, out); err != nil {
		return err
	}
	if in.Token == nil {
		return nil
	}
	out.Data = in.Token
	sqlx, err := sess.Db()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if in.Token.Has == nil {
		in.Token.Has = &TokenHas{}
	}
	if in.CurToken == nil {
		in.Token.SetCreatedAt(now)
		return sqlx.Insert("user_oauth_token", in.Token)
	}
	in.Token.Has.UserID = true
	in.Token.Has.Provider = true
	in.Token.Has.EncToken = true
	in.Token.SetUpdatedAt(now)
	return sqlx.Update("user_oauth_token", in.Token)
}
