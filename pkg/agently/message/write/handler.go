package write

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"unicode/utf8"

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
	if len(out.Violations) > 0 { //TODO better error hanlding
		out.setError(fmt.Errorf("failed validation"))
		return out, response.NewError(http.StatusBadRequest, "bad request"+" - failed validation: "+out.Violations[0].Message)
	}
	return out, nil
}

func (h *Handler) exec(ctx context.Context, sess handler.Session, out *Output) error {
	in := &Input{}
	if err := in.Init(ctx, sess, out); err != nil {
		return err
	}
	out.Data = in.Messages
	if err := in.Validate(ctx, sess, out); err != nil || len(out.Violations) > 0 {
		return err
	}
	sql, err := sess.Db()
	if err != nil {
		return err
	}
	const maxContentBytes = 64 * 1024
	for _, rec := range in.Messages {
		// Truncate content to maxContentBytes preserving valid UTF-8
		if rec != nil {
			b := []byte(rec.Content)
			if len(b) > maxContentBytes {
				trunc := b[:maxContentBytes]
				// ensure we don't cut a multi-byte rune; backtrack to a valid boundary
				for !utf8.Valid(trunc) && len(trunc) > 0 {
					trunc = trunc[:len(trunc)-1]
				}
				rec.Content = string(trunc)
				if rec.Has != nil {
					rec.Has.Content = true
				}
			}
		}
		if _, ok := in.CurMessageById[rec.Id]; !ok {
			if err = sql.Insert("message", rec); err != nil {
				return err
			}
		} else {
			if err = sql.Update("message", rec); err != nil {
				return err
			}
		}
	}
	return nil
}
