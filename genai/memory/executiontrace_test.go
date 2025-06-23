package memory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestExecutionStore_ListByParent verifies that ListByParent correctly filters
// execution traces by ParentMsgID while preserving the order of insertion.
func TestExecutionStore_ListByParent(t *testing.T) {
	ctx := context.Background()
	store := NewExecutionStore()

	convID := "conv1"

	// Prepare input traces – intentionally mix parent IDs.
	inputs := []*ExecutionTrace{
		{ParentMsgID: "1", Name: "tool.alpha"},
		{ParentMsgID: "1", Name: "tool.beta"},
		{ParentMsgID: "2", Name: "tool.gamma"},
	}

	// Persist traces.
	for _, tr := range inputs {
		_, err := store.Add(ctx, convID, tr)
		assert.NoError(t, err)
	}

	testCases := []struct {
		name        string
		parentMsgID string
		expected    []*ExecutionTrace
	}{
		{
			name:        "parent 1 returns two traces",
			parentMsgID: "1",
			expected:    []*ExecutionTrace{inputs[0], inputs[1]},
		},
		{
			name:        "parent 2 returns single trace",
			parentMsgID: "2",
			expected:    []*ExecutionTrace{inputs[2]},
		},
		{
			name:        "unknown parent returns empty slice",
			parentMsgID: "3",
			expected:    []*ExecutionTrace{},
		},
	}

	for _, tc := range testCases {
		actual, err := store.ListByParent(ctx, convID, tc.parentMsgID)
		assert.NoError(t, err, tc.name)
		assert.EqualValues(t, tc.expected, actual, tc.name)
	}
}
