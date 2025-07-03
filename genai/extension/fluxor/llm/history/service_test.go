package history

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/genai/memory"
)

func TestService_Basic(t *testing.T) {
	hist := memory.NewHistoryStore()
	svc := New(hist, nil)

	ctx := context.Background()

	// 1. add two messages
	addCases := []AddMessageInput{
		{ConversationID: "c1", Role: "user", Content: "hi"},
		{ConversationID: "c1", Role: "assistant", Content: "hello"},
	}
	for _, in := range addCases {
		var out AddMessageOutput
		err := svc.addMessage(ctx, &in, &out)
		assert.NoError(t, err)
		assert.NotEmpty(t, out.ID)
	}

	// 2. fetch all messages
	var outAll MessagesOutput
	err := svc.messages(ctx, &MessagesInput{ConversationID: "c1"}, &outAll)
	assert.NoError(t, err)
	assert.EqualValues(t, 2, len(outAll.Messages))

	// 3. fetch last N=1
	var outLast LastNOutput
	err = svc.lastN(ctx, &LastNInput{ConversationID: "c1", N: 1}, &outLast)
	assert.NoError(t, err)
	assert.EqualValues(t, 1, len(outLast.Messages))
	assert.Equal(t, "assistant", outLast.Messages[0].Role)
}

func TestService_Compact(t *testing.T) {
	hist := memory.NewHistoryStore()

	// seed 6 messages, one with attachment at index 2
	conv := "c1"
	for i := 0; i < 6; i++ {
		msg := memory.Message{ID: uuid.New().String(), ConversationID: conv, Role: "user", Content: fmt.Sprintf("msg%d", i)}
		if i == 2 {
			msg.Attachments = memory.Attachments{{Name: "f.txt", URL: "file://"}}
		}
		_ = hist.AddMessage(context.Background(), msg)
	}

	// create service with nil llm (we stub summary via direct Update)
	svc := New(hist, nil)

	// hack: set llm nil but expect compact to short-circuit; should error
	var cerr error
	func() {
		var out CompactOutput
		cerr = svc.compact(context.Background(), &CompactInput{ConversationID: conv, Threshold: 4, LastN: 2}, &out)
	}()
	assert.Error(t, cerr) // llm core nil
}
