package write

import (
	"context"
	"errors"
	"fmt"
	"net/http"

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
	if in.Session == nil {
		return nil
	}
	out.Data = in.Session
	sqlx, err := sess.Db()
	if err != nil {
		return err
	}
	if in.CurSession == nil {
		return sqlx.Insert("session", in.Session)
	}
	return sqlx.Update("session", in.Session)
}
