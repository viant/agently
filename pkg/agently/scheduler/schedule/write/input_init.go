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
			if strings.TrimSpace(rec.Visibility) == "" {
				rec.SetVisibility("private")
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

		// If schedule-defining attributes change, clear next_run_at so the scheduler can recompute it.
		if cur := i.CurScheduleById[rec.Id]; cur != nil {
			clearNextRunAtOnScheduleChange(rec, cur)
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

func clearNextRunAtOnScheduleChange(rec *Schedule, cur *Schedule) {
	if rec == nil || cur == nil || rec.Has == nil {
		return
	}

	changed := false

	if rec.Has.StartAt && !timePtrEqual(rec.StartAt, cur.StartAt) {
		changed = true
	}
	if rec.Has.EndAt && !timePtrEqual(rec.EndAt, cur.EndAt) {
		changed = true
	}
	if rec.Has.CronExpr && !stringPtrEqual(rec.CronExpr, cur.CronExpr) {
		changed = true
	}
	if rec.Has.IntervalSeconds && !intPtrEqual(rec.IntervalSeconds, cur.IntervalSeconds) {
		changed = true
	}
	if rec.Has.Timezone && strings.TrimSpace(rec.Timezone) != strings.TrimSpace(cur.Timezone) {
		changed = true
	}
	if rec.Has.ScheduleType && strings.TrimSpace(rec.ScheduleType) != strings.TrimSpace(cur.ScheduleType) {
		changed = true
	}

	if !changed {
		return
	}

	rec.NextRunAt = nil
	rec.ensureHas()
	rec.Has.NextRunAt = true
}

func timePtrEqual(a *time.Time, b *time.Time) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Equal(*b)
}

func stringPtrEqual(a *string, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return strings.TrimSpace(*a) == strings.TrimSpace(*b)
}

func intPtrEqual(a *int, b *int) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
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
