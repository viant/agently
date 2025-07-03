package memory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHistoryStore_MessagesSince(t *testing.T) {
	ctx := context.Background()
	convID := "conv1"

	// Prepare store with three messages
	store := NewHistoryStore()

	msgs := []Message{
		{ID: "m1", ConversationID: convID, Role: "user"},
		{ID: "m2", ConversationID: convID, Role: "assistant"},
		{ID: "m3", ConversationID: convID, Role: "system"},
	}

	for _, m := range msgs {
		_ = store.AddMessage(ctx, m)
	}

	testCases := []struct {
		name        string
		sinceID     string
		expectedIDs []string
	}{
		{name: "full when empty", sinceID: "", expectedIDs: []string{"m1", "m2", "m3"}},
		{name: "from middle", sinceID: "m2", expectedIDs: []string{"m2", "m3"}},
		{name: "from last", sinceID: "m3", expectedIDs: []string{"m3"}},
		{name: "not found", sinceID: "missing", expectedIDs: []string{}},
	}

	for _, tc := range testCases {
		actual, err := store.MessagesSince(ctx, convID, tc.sinceID)
		assert.NoError(t, err, tc.name)

		actualIDs := make([]string, 0, len(actual))
		for _, m := range actual {
			actualIDs = append(actualIDs, m.ID)
		}

		assert.EqualValues(t, tc.expectedIDs, actualIDs, tc.name)
	}
}
