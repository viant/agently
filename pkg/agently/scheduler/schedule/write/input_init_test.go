package write

import (
	"context"
	"testing"

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
