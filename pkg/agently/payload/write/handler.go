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
	out.Data = in.Payloads
	if err := in.Validate(ctx, sess, out); err != nil || len(out.Violations) > 0 {
		return err
	}
	sql, err := sess.Db()
	if err != nil {
		return err
	}
	for _, rec := range in.Payloads {
		if _, ok := in.CurByID[rec.Id]; !ok {
			if err = sql.Insert("call_payload", rec); err != nil {
				return err
			}
		} else {
			if err = sql.Update("call_payload", rec); err != nil {
				return err
			}
		}
	}
	return nil
}
