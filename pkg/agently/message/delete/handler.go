package delete

import (
	"context"
	"errors"
	"net/http"

	"github.com/viant/xdatly/handler"
	"github.com/viant/xdatly/handler/response"
)

type Handler struct{}

func (h *Handler) Exec(ctx context.Context, sess handler.Session) (interface{}, error) {
	output := &Output{}
	output.Status.Status = "ok"
	if err := h.exec(ctx, sess, output); err != nil {
		var respErr *response.Error
		if errors.As(err, &respErr) {
			return output, err
		}
		output.setError(err)
	}
	if len(output.Violations) > 0 {
		output.setError(errors.New("failed validation"))
		return output, response.NewError(http.StatusBadRequest, "bad request")
	}
	return output, nil
}

func (h *Handler) exec(ctx context.Context, sess handler.Session, output *Output) error {
	in := &Input{}
	if err := sess.Stater().Bind(ctx, in); err != nil {
		return err
	}
	if len(in.Ids) == 0 {
		return nil
	}
	sql, err := sess.Db()
	if err != nil {
		return err
	}
	// Best-effort delete; rely on DB FKs (CASCADE) for dependent rows
	for _, id := range in.Ids {
		if err := sql.Delete("message", struct {
			Id string `sqlx:"id,primaryKey"`
		}{Id: id}); err != nil {
			return err
		}
		output.Data = append(output.Data, id)
	}
	return nil
}
