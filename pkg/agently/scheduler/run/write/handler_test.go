package write

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/viant/agently/internal/testutil/dbtest"
	"github.com/viant/xdatly/handler/response"
	_ "modernc.org/sqlite"
)

func TestHandler_Exec_RunWrite(t *testing.T) {
	type expected struct {
		status       string
		expectErr    bool
		expectErr400 bool
		expectRows   int
		createdSet   bool
		updatedSet   bool
	}

	type testCase struct {
		name      string
		input     *Input
		seed      func(t *testing.T, db *sql.DB)
		validator *fakeValidator
		expect    expected
	}

	now := time.Now().UTC().Add(-2 * time.Hour)

	cases := []testCase{
		{
			name: "insert sets created_at",
			input: &Input{Runs: []*Run{{
				Id:               "r1",
				ScheduleId:       "s1",
				Status:           "pending",
				ConversationKind: "scheduled",
			}}},
			seed: func(t *testing.T, db *sql.DB) {
				t.Helper()
				_, err := db.Exec(
					`INSERT INTO schedule (id, name, agent_ref, enabled, schedule_type, timezone, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
					"s1", "sched-1", "agent-1", 1, "cron", "UTC", time.Now().UTC(),
				)
				require.NoError(t, err)
			},
			expect: expected{status: "ok", expectRows: 1, createdSet: true},
		},
		{
			name: "update sets updated_at",
			input: &Input{
				Runs: []*Run{{
					Id:               "r2",
					ScheduleId:       "s2",
					Status:           "running",
					ConversationKind: "scheduled",
					CreatedAt:        &now,
				}},
				CurRun: []*Run{{Id: "r2"}},
			},
			seed: func(t *testing.T, db *sql.DB) {
				t.Helper()
				_, err := db.Exec(
					`INSERT INTO schedule (id, name, agent_ref, enabled, schedule_type, timezone, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
					"s2", "sched-2", "agent-2", 1, "cron", "UTC", time.Now().UTC(),
				)
				require.NoError(t, err)
				_, err = db.Exec(
					`INSERT INTO schedule_run (id, schedule_id, created_at, status, conversation_kind) VALUES (?, ?, ?, ?, ?)`,
					"r2", "s2", now, "pending", "scheduled",
				)
				require.NoError(t, err)
			},
			expect: expected{status: "ok", expectRows: 1, createdSet: true, updatedSet: true},
		},
		{
			name: "validation violation prevents write",
			input: &Input{Runs: []*Run{{
				Id:               "r3",
				ScheduleId:       "s3",
				Status:           "pending",
				ConversationKind: "scheduled",
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
			db, _, cleanup := dbtest.CreateTempSQLiteDB(t, "agently-run-write")
			t.Cleanup(cleanup)
			seedRunSchema(t, db)

			if tc.seed != nil {
				tc.seed(t, db)
			}

			v := tc.validator
			if v == nil {
				v = &fakeValidator{}
			}
			sess := newRunSession(tc.input, db, v)

			outAny, err := (&Handler{}).Exec(context.Background(), sess)
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

			count := countRuns(t, db)
			require.Equal(t, tc.expect.expectRows, count)
			if tc.expect.expectRows > 0 {
				row := fetchRunRow(t, db, tc.input.Runs[0].Id)
				if tc.expect.createdSet {
					require.True(t, row.createdAt.Valid)
				}
				if tc.expect.updatedSet {
					require.True(t, row.updatedAt.Valid)
				} else {
					require.False(t, row.updatedAt.Valid)
				}
			}
		})
	}
}

type runRow struct {
	createdAt sql.NullString
	updatedAt sql.NullString
}

func fetchRunRow(t *testing.T, db *sql.DB, id string) runRow {
	t.Helper()
	row := runRow{}
	err := db.QueryRow(`SELECT created_at, updated_at FROM schedule_run WHERE id = ?`, id).Scan(
		&row.createdAt, &row.updatedAt,
	)
	require.NoError(t, err)
	return row
}

func countRuns(t *testing.T, db *sql.DB) int {
	t.Helper()
	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(1) FROM schedule_run`).Scan(&count))
	return count
}
