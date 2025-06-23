package memory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHistoryStore_AddMessage_Deduplicate(t *testing.T) {
	ctx := context.Background()
	store := NewHistoryStore()

	convID := "conv1"

	msg1 := Message{
		ID:             "msg-123",
		ConversationID: convID,
		Role:           "policyapproval",
		Status:         "open",
	}

	msg2 := msg1 // same ID and convID – represents duplicate event

	// Add first time.
	assert.NoError(t, store.AddMessage(ctx, msg1))
	// Add duplicate.
	assert.NoError(t, store.AddMessage(ctx, msg2))

	msgs, err := store.GetMessages(ctx, convID)
	assert.NoError(t, err)

	expected := []Message{msg1}
	// CreatedAt is assigned during AddMessage when zero value is provided.
	expected[0].CreatedAt = msgs[0].CreatedAt

	assert.EqualValues(t, expected, msgs)
}
