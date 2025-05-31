package conversation

import (
	"context"
	genai "github.com/viant/agently/genai/agent"
	agentpkg "github.com/viant/agently/genai/extension/fluxor/llm/agent"
	"testing"

	"github.com/stretchr/testify/assert"
)

// stubHandler is a minimal QueryHandler used in tests. It echoes the query
// back as content so that we can validate the manager wiring without bringing
// up the full agent orchestration.
func stubHandler(_ context.Context, in *agentpkg.QueryInput, out *agentpkg.QueryOutput) error {
	out.Content = "echo: " + in.Query
	return nil
}

func TestManager_Accept(t *testing.T) {
	testCases := []struct {
		name         string
		convID       string
		expectedIDFn func(string) bool // helper to validate generated IDs
	}{
		{
			name:   "generate new ID when missing",
			convID: "",
			expectedIDFn: func(id string) bool {
				return id != "" // any non-empty ID is acceptable
			},
		},
		{
			name:   "preserve provided conversation ID",
			convID: "conversation-123",
			expectedIDFn: func(id string) bool {
				return id == "conversation-123"
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mgr := New(nil, stubHandler) // default in-memory history & uuid generator

			input := &agentpkg.QueryInput{
				ConversationID: tc.convID,
				Agent:          &genai.Agent{},
				Query:          "hello",
			}

			got, err := mgr.Accept(context.Background(), input)
			assert.NoError(t, err)

			// Validate conversation ID rule
			assert.True(t, tc.expectedIDFn(input.ConversationID))

			// Validate handler output propagated
			expectedOutput := &agentpkg.QueryOutput{Content: "echo: hello"}
			assert.EqualValues(t, expectedOutput, got)
		})
	}
}
