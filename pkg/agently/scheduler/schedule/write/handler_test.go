package write

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	authctx "github.com/viant/agently/internal/auth"
	"github.com/viant/agently/internal/testutil/dbtest"
	"github.com/viant/xdatly/handler/response"
	_ "modernc.org/sqlite"
)

func TestHandler_Exec_ScheduleWrite(t *testing.T) {
	type expected struct {
		status       string
		expectErr    bool
		expectErr400 bool
		expectRows   int
		createdSet   bool
		updatedSet   bool
		createdBy    string
	}

	type testCase struct {
		name      string
		ctx       context.Context
		input     *Input
		seed      func(t *testing.T, db *sql.DB)
		validator *fakeValidator
		expect    expected
	}

	now := time.Now().UTC().Add(-2 * time.Hour)

	cases := []testCase{
		{
			name: "insert sets created_at",
			input: &Input{Schedules: []*Schedule{{
				Name:           "daily",
				AgentRef:       "agent",
				Enabled:        true,
				ScheduleType:   "adhoc",
				Timezone:       "UTC",
				TimeoutSeconds: 123,
			}}},
			expect: expected{status: "ok", expectRows: 1, createdSet: true},
		},
		{
			name: "insert sets created_by_user_id",
			ctx:  authctx.WithUserInfo(context.Background(), &authctx.UserInfo{Subject: "ppoudyal"}),
			input: &Input{Schedules: []*Schedule{{
				Name:           "daily2",
				AgentRef:       "agent",
				Enabled:        true,
				ScheduleType:   "adhoc",
				Timezone:       "UTC",
				TimeoutSeconds: 123,
			}}},
			expect: expected{status: "ok", expectRows: 1, createdSet: true, createdBy: "ppoudyal"},
		},
		{
			name: "update sets updated_at",
			input: &Input{
				Schedules: []*Schedule{{
					Id:             "sched-1",
					Name:           "weekly",
					AgentRef:       "agent",
					Enabled:        true,
					ScheduleType:   "cron",
					Timezone:       "UTC",
					TimeoutSeconds: 456,
					CreatedAt:      &now,
				}},
				CurSchedule: []*Schedule{{Id: "sched-1"}},
			},
			seed: func(t *testing.T, db *sql.DB) {
				t.Helper()
				_, err := db.Exec(
					`INSERT INTO schedule (id, name, agent_ref, enabled, schedule_type, timezone, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
					"sched-1", "weekly", "agent", 1, "cron", "UTC", now,
				)
				require.NoError(t, err)
			},
			expect: expected{status: "ok", expectRows: 1, createdSet: true, updatedSet: true},
		},
		{
			name: "validation violation prevents write",
			input: &Input{Schedules: []*Schedule{{
				Id:           "sched-2",
				Name:         "monthly",
				AgentRef:     "agent",
				Enabled:      true,
				ScheduleType: "adhoc",
				Timezone:     "UTC",
			}}},
			validator: &fakeValidator{violate: true},
			expect: expected{
				status:       "error",
				expectErr:    true,
				expectErr400: true,
				expectRows:   0,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, _, cleanup := dbtest.CreateTempSQLiteDB(t, "agently-schedule-write")
			t.Cleanup(cleanup)
			seedScheduleSchema(t, db)

			if tc.seed != nil {
				tc.seed(t, db)
			}

			v := tc.validator
			if v == nil {
				v = &fakeValidator{}
			}
			sess := newScheduleSession(tc.input, db, v)

			callCtx := tc.ctx
			if callCtx == nil {
				callCtx = context.Background()
			}
			outAny, err := (&Handler{}).Exec(callCtx, sess)
			if tc.expect.expectErr {
				require.Error(t, err)
				if tc.expect.expectErr400 {
					var rErr *response.Error
					require.True(t, errors.As(err, &rErr))
					require.Equal(t, 400, rErr.StatusCode())
				}
			} else {
				require.NoError(t, err)
			}

			out, ok := outAny.(*Output)
			require.True(t, ok)
			require.Equal(t, tc.expect.status, out.Status.Status)

			count := countSchedules(t, db)
			require.Equal(t, tc.expect.expectRows, count)
			if tc.expect.expectRows > 0 {
				row := fetchScheduleRow(t, db, tc.input.Schedules[0].Id)
				if tc.expect.createdSet {
					require.True(t, row.createdAt.Valid)
				}
				if tc.expect.updatedSet {
					require.True(t, row.updatedAt.Valid)
				} else {
					require.False(t, row.updatedAt.Valid)
				}
				if tc.expect.createdBy != "" {
					require.Equal(t, tc.expect.createdBy, row.createdByUserID.String)
				}
				require.Equal(t, int64(tc.input.Schedules[0].TimeoutSeconds), row.timeoutSeconds)
			}
		})
	}
}

type scheduleRow struct {
	createdAt       sql.NullString
	updatedAt       sql.NullString
	timeoutSeconds  int64
	createdByUserID sql.NullString
}

func fetchScheduleRow(t *testing.T, db *sql.DB, id string) scheduleRow {
	t.Helper()
	row := scheduleRow{}
	err := db.QueryRow(`SELECT created_at, updated_at, timeout_seconds, created_by_user_id FROM schedule WHERE id = ?`, id).Scan(
		&row.createdAt, &row.updatedAt, &row.timeoutSeconds, &row.createdByUserID,
	)
	require.NoError(t, err)
	return row
}

func countSchedules(t *testing.T, db *sql.DB) int {
	t.Helper()
	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(1) FROM schedule`).Scan(&count))
	return count
}
