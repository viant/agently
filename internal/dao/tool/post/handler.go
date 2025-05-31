package post

import (
	"context"
	"errors"
	"fmt"
	"github.com/viant/agently/pkg/dependency/checksum"
	"github.com/viant/xdatly/handler"
	"github.com/viant/xdatly/handler/response"
	"github.com/viant/xdatly/types/core"
	"net/http"
	"reflect"
)

func init() {
	core.RegisterType(PackageName, "Handler", reflect.TypeOf(Handler{}), checksum.GeneratedTime)
}

type Handler struct{}

func (h *Handler) Exec(ctx context.Context, sess handler.Session) (interface{}, error) {
	output := &Output{}
	output.Status.Status = "ok"
	err := h.exec(ctx, sess, output)
	if err != nil {
		var responseError *response.Error
		if errors.As(err, &responseError) {
			return output, err
		}
		output.setError(err)
	}
	if len(output.Violations) > 0 {
		output.setError(fmt.Errorf("failed validation"))
		return output, response.NewError(http.StatusBadRequest, "bad request")
	}
	return output, nil
}

func (h *Handler) exec(ctx context.Context, sess handler.Session, output *Output) error {
	input := &Input{}
	if err := input.Init(ctx, sess, output); err != nil {
		return err
	}
	output.Data = input.ToolCall
	if err := input.Validate(ctx, sess, output); err != nil || len(output.Violations) > 0 {
		return err
	}
	sql, err := sess.Db()
	if err != nil {
		return err
	}
	sequencer := sql
	toolCall := input.ToolCall

	if err = sequencer.Allocate(ctx, "tool_call", input.ToolCall, "Id"); err != nil {
		return err
	}

	for _, recTool := range toolCall {
		if err = sql.Insert("tool_call", recTool); err != nil {
			return err
		}
	}
	return nil
}
