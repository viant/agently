package memory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// stub summarizer that returns a fixed summary message.
func stubSummarizer(content string) SummarizerFunc {
	return func(ctx context.Context, _ []Message) (Message, error) {
		return Message{Role: "system", Content: content}, nil
	}
}

func TestVisibilityPolicy(t *testing.T) {
	messages := []Message{{Role: "user", Content: "hello"}, {Role: "assistant", Content: "hi"}}

	testCases := []struct {
		name       string
		mode       string
		summarizer SummarizerFunc
		expected   []Message
	}{
		{
			name:       "full returns all messages",
			mode:       "full",
			summarizer: nil,
			expected:   messages,
		},
		{
			name:       "none hides messages",
			mode:       "none",
			summarizer: nil,
			expected:   []Message{},
		},
		{
			name:       "summary collapses to single message",
			mode:       "summary",
			summarizer: stubSummarizer("SUM"),
			expected:   []Message{{Role: "system", Content: "SUM"}},
		},
		{
			name:       "unknown mode behaves like full",
			mode:       "unknown",
			summarizer: nil,
			expected:   messages,
		},
	}

	ctx := context.Background()
	for _, tc := range testCases {
		p := NewVisibilityPolicy(tc.mode, tc.summarizer)
		out, err := p.Apply(ctx, messages)
		assert.NoError(t, err, tc.name)
		assert.EqualValues(t, tc.expected, out, tc.name)
	}
}
