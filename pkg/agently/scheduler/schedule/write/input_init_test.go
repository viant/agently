package write

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/viant/agently/internal/testutil/dbtest"
	_ "modernc.org/sqlite"
)

func TestInput_Init(t *testing.T) {
	tests := []struct {
		name           string
		input          *Input
		expectIndexed  bool
		expectID       bool
		expectTimezone string
		expectSchedule string
		expectCreated  bool
		expectUpdated  bool
	}{
		{
			name: "insert assigns id, timezone, schedule_type",
			input: &Input{Schedules: []*Schedule{{
				Name:         "daily",
				AgentRef:     "agent",
				Enabled:      true,
				ScheduleType: "",
				Timezone:     "",
			}}},
			expectIndexed:  true,
			expectID:       true,
			expectTimezone: "UTC",
			expectSchedule: "adhoc",
			expectCreated:  true,
		},
		{
			name: "update keeps existing values",
			input: &Input{
				Schedules: []*Schedule{{
					Id:           "sched-1",
					Name:         "weekly",
					AgentRef:     "agent",
					Enabled:      true,
					ScheduleType: "cron",
					Timezone:     "UTC",
				}},
				CurSchedule: []*Schedule{{Id: "sched-1"}},
			},
			expectIndexed:  true,
			expectID:       true,
			expectSchedule: "cron",
			expectTimezone: "UTC",
			expectUpdated:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _, cleanup := dbtest.CreateTempSQLiteDB(t, "agently-schedule-init")
			t.Cleanup(cleanup)
			seedScheduleSchema(t, db)
			sess := newScheduleSession(tt.input, db, &fakeValidator{})

			err := tt.input.Init(context.Background(), sess, &Output{})
			require.NoError(t, err)
			if tt.expectIndexed {
				require.NotNil(t, tt.input.CurScheduleById)
				if len(tt.input.CurSchedule) > 0 {
					_, ok := tt.input.CurScheduleById[tt.input.CurSchedule[0].Id]
					require.True(t, ok)
				}
			}
			require.Len(t, tt.input.Schedules, 1)
			s := tt.input.Schedules[0]
			if tt.expectID {
				require.NotEmpty(t, s.Id)
			}
			require.Equal(t, tt.expectTimezone, s.Timezone)
			require.Equal(t, tt.expectSchedule, s.ScheduleType)
			if tt.expectCreated {
				require.NotNil(t, s.CreatedAt)
			}
			if tt.expectUpdated {
				require.NotNil(t, s.UpdatedAt)
			}
		})
	}
}

func TestInput_Init_ClearsNextRunAtOnScheduleChange(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	start1 := base.Add(-2 * time.Hour)
	start2 := base.Add(-1 * time.Hour)
	end1 := base.Add(2 * time.Hour)
	end2 := base.Add(3 * time.Hour)
	cron1 := "*/1 * * * *"
	cron2 := "*/2 * * * *"
	interval1 := 60
	interval2 := 120

	tests := []struct {
		name   string
		cur    *Schedule
		update func(rec *Schedule)
	}{
		{
			name: "start_at change clears next_run_at",
			cur:  &Schedule{Id: "sched-1", Name: "n", AgentRef: "agent", Enabled: true, ScheduleType: "cron", Timezone: "UTC", StartAt: &start1, NextRunAt: &base},
			update: func(rec *Schedule) {
				rec.SetStartAt(start2)
			},
		},
		{
			name: "end_at change clears next_run_at",
			cur:  &Schedule{Id: "sched-1", Name: "n", AgentRef: "agent", Enabled: true, ScheduleType: "cron", Timezone: "UTC", EndAt: &end1, NextRunAt: &base},
			update: func(rec *Schedule) {
				rec.SetEndAt(end2)
			},
		},
		{
			name: "cron_expr change clears next_run_at",
			cur:  &Schedule{Id: "sched-1", Name: "n", AgentRef: "agent", Enabled: true, ScheduleType: "cron", Timezone: "UTC", CronExpr: &cron1, NextRunAt: &base},
			update: func(rec *Schedule) {
				rec.SetCronExpr(cron2)
			},
		},
		{
			name: "interval_seconds change clears next_run_at",
			cur:  &Schedule{Id: "sched-1", Name: "n", AgentRef: "agent", Enabled: true, ScheduleType: "interval", Timezone: "UTC", IntervalSeconds: &interval1, NextRunAt: &base},
			update: func(rec *Schedule) {
				rec.SetIntervalSeconds(interval2)
			},
		},
		{
			name: "timezone change clears next_run_at",
			cur:  &Schedule{Id: "sched-1", Name: "n", AgentRef: "agent", Enabled: true, ScheduleType: "cron", Timezone: "UTC", CronExpr: &cron1, NextRunAt: &base},
			update: func(rec *Schedule) {
				rec.SetTimezone("America/Los_Angeles")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _, cleanup := dbtest.CreateTempSQLiteDB(t, "agently-schedule-init-next-run")
			t.Cleanup(cleanup)
			seedScheduleSchema(t, db)

			rec := &Schedule{
				Id:           "sched-1",
				Name:         "n",
				AgentRef:     "agent",
				Enabled:      true,
				ScheduleType: tt.cur.ScheduleType,
				Timezone:     tt.cur.Timezone,
			}
			tt.update(rec)
			require.NotNil(t, rec.Has, "expected Has markers to be set by Set* methods")
			require.False(t, rec.Has.NextRunAt)

			in := &Input{
				Schedules:   []*Schedule{rec},
				CurSchedule: []*Schedule{tt.cur},
			}
			sess := newScheduleSession(in, db, &fakeValidator{})

			err := in.Init(context.Background(), sess, &Output{})
			require.NoError(t, err)

			got := in.Schedules[0]
			require.NotNil(t, got.Has)
			require.True(t, got.Has.NextRunAt)
			require.Nil(t, got.NextRunAt)
		})
	}
}

func TestInput_Init_DoesNotClearNextRunAtWhenScheduleAttrsUnchanged(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cron := "*/1 * * * *"

	db, _, cleanup := dbtest.CreateTempSQLiteDB(t, "agently-schedule-init-next-run-unchanged")
	t.Cleanup(cleanup)
	seedScheduleSchema(t, db)

	cur := &Schedule{Id: "sched-1", Name: "n", AgentRef: "agent", Enabled: true, ScheduleType: "cron", Timezone: "UTC", CronExpr: &cron, NextRunAt: &base}
	rec := &Schedule{
		Id:           "sched-1",
		Name:         "n",
		AgentRef:     "agent",
		Enabled:      true,
		ScheduleType: "cron",
		Timezone:     "UTC",
	}
	rec.SetCronExpr(cron) // Has.CronExpr=true but value is identical to current

	in := &Input{
		Schedules:   []*Schedule{rec},
		CurSchedule: []*Schedule{cur},
	}
	sess := newScheduleSession(in, db, &fakeValidator{})

	err := in.Init(context.Background(), sess, &Output{})
	require.NoError(t, err)

	got := in.Schedules[0]
	require.NotNil(t, got.Has)
	require.False(t, got.Has.NextRunAt)
}
