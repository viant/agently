package write

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	authctx "github.com/viant/agently/internal/auth"
	"github.com/viant/xdatly/handler"
)

func (i *Input) Init(ctx context.Context, sess handler.Session, _ *Output) error {
	if err := sess.Stater().Bind(ctx, i); err != nil {
		return err
	}
	i.indexSlice()

	// Ensure IDs for new schedules prior to validation
	now := time.Now().UTC()
	userID := strings.TrimSpace(authctx.EffectiveUserID(ctx))
	for _, rec := range i.Schedules {
		if rec == nil {
			continue
		}
		_, isUpdate := i.CurScheduleById[rec.Id]
		isInsert := !isUpdate
		if isInsert {
			if strings.TrimSpace(rec.Id) == "" {
				rec.SetId(uuid.NewString())
			}
			if strings.TrimSpace(strPtrValue(rec.CreatedByUserID)) == "" && userID != "" {
				rec.SetCreatedByUserID(userID)
			}
			if rec.Timezone == "" {
				rec.Timezone = "UTC"
			}
			if rec.Has == nil || !rec.Has.ScheduleType {
				rec.SetScheduleType("adhoc")
			}
			if rec.CreatedAt == nil {
				rec.SetCreatedAt(now)
			}
			continue
		}
		// Backfill owner for legacy schedules when missing (do not overwrite).
		if cur := i.CurScheduleById[rec.Id]; cur != nil &&
			strings.TrimSpace(strPtrValue(cur.CreatedByUserID)) == "" &&
			strings.TrimSpace(strPtrValue(rec.CreatedByUserID)) == "" &&
			userID != "" {
			rec.SetCreatedByUserID(userID)
		}
		if rec.UpdatedAt == nil {
			rec.SetUpdatedAt(now)
		}
	}

	return nil
}

func strPtrValue(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (i *Input) indexSlice() {
	i.CurScheduleById = map[string]*Schedule{}
	for _, m := range i.CurSchedule {
		if m != nil {
			i.CurScheduleById[m.Id] = m
		}
	}
}
