package write

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"strings"

	"github.com/google/uuid"
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
	// Ensure IDs for new schedules prior to validation
	for _, rec := range in.Schedules {
		if rec == nil {
			continue
		}
		if strings.TrimSpace(rec.Id) == "" {
			rec.SetId(uuid.NewString())
			if rec.Timezone == "" {
				rec.Timezone = "UTC"
			}
			if rec.Enabled == nil {
				rec.SetEnabled(0)
			}
		}
	}
	out.Data = in.Schedules
	if err := in.Validate(ctx, sess, out); err != nil || len(out.Violations) > 0 {
		return err
	}

	sqlx, err := sess.Db()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, rec := range in.Schedules {
		if rec == nil {
			continue
		}
		if _, ok := in.CurScheduleById[rec.Id]; !ok {
			if rec.CreatedAt == nil {
				rec.SetCreatedAt(now)
			}
			if err = sqlx.Insert("schedule", rec); err != nil {
				return err
			}
		} else {
			if rec.UpdatedAt == nil {
				rec.SetUpdatedAt(now)
			}
			if err = sqlx.Update("schedule", rec); err != nil {
				return err
			}
		}
	}
	return nil
}
