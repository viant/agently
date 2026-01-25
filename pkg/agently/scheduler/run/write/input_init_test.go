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
		name          string
		input         *Input
		expectIndexed bool
		expectCreated bool
		expectUpdated bool
	}{
		{
			name: "insert stamps created_at",
			input: &Input{Runs: []*Run{{
				Id:               "r1",
				ScheduleId:       "s1",
				Status:           "pending",
				ConversationKind: "scheduled",
			}}},
			expectIndexed: true,
			expectCreated: true,
		},
		{
			name: "update stamps updated_at",
			input: &Input{
				Runs:   []*Run{{Id: "r2", ScheduleId: "s2", Status: "running", ConversationKind: "scheduled"}},
				CurRun: []*Run{{Id: "r2"}},
			},
			expectIndexed: true,
			expectUpdated: true,
		},
		{
			name:  "nil runs no-op",
			input: &Input{Runs: nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _, cleanup := dbtest.CreateTempSQLiteDB(t, "agently-run-init")
			t.Cleanup(cleanup)
			seedRunSchema(t, db)
			sess := newRunSession(tt.input, db, &fakeValidator{})
			err := tt.input.Init(context.Background(), sess, &Output{})
			require.NoError(t, err)
			if tt.expectIndexed {
				require.NotNil(t, tt.input.CurRunById)
				if len(tt.input.CurRun) > 0 {
					_, ok := tt.input.CurRunById[tt.input.CurRun[0].Id]
					require.True(t, ok)
				}
			}
			if len(tt.input.Runs) == 0 {
				return
			}
			r := tt.input.Runs[0]
			if tt.expectCreated {
				require.NotNil(t, r.CreatedAt)
			}
			if tt.expectUpdated {
				require.NotNil(t, r.UpdatedAt)
			}
		})
	}
}
