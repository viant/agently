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
			name:             "no runs no violations",
			input:            &Input{Runs: nil},
			validator:        &fakeValidator{},
			expectViolations: 0,
		},
		{
			name:             "violation appended",
			input:            &Input{Runs: []*Run{{Id: "r1", ScheduleId: "s1", Status: "pending", ConversationKind: "scheduled"}}},
			validator:        &fakeValidator{violate: true},
			expectViolations: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _, cleanup := dbtest.CreateTempSQLiteDB(t, "agently-run-validate")
			t.Cleanup(cleanup)
			seedRunSchema(t, db)
			sess := newRunSession(tt.input, db, tt.validator)
			out := &Output{}
			err := tt.input.Validate(context.Background(), sess, out)
			require.NoError(t, err)
			require.Len(t, out.Violations, tt.expectViolations)
		})
	}
}
