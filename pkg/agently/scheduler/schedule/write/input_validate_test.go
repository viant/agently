package write

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/viant/agently/internal/testutil/dbtest"
	_ "modernc.org/sqlite"
)

func TestInput_Validate(t *testing.T) {
	tests := []struct {
		name             string
		input            *Input
		validator        *fakeValidator
		expectViolations int
	}{
		{
			name:             "no schedules no violations",
			input:            &Input{Schedules: nil},
			validator:        &fakeValidator{},
			expectViolations: 0,
		},
		{
			name:             "violation appended",
			input:            &Input{Schedules: []*Schedule{{Id: "s1", Name: "daily", AgentRef: "agent", Enabled: true, ScheduleType: "adhoc", Timezone: "UTC"}}},
			validator:        &fakeValidator{violate: true},
			expectViolations: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _, cleanup := dbtest.CreateTempSQLiteDB(t, "agently-schedule-validate")
			t.Cleanup(cleanup)
			seedScheduleSchema(t, db)
			sess := newScheduleSession(tt.input, db, tt.validator)
			out := &Output{}
			err := tt.input.Validate(context.Background(), sess, out)
			require.NoError(t, err)
			require.Len(t, out.Violations, tt.expectViolations)
		})
	}
}
