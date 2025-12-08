package conversation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMessage_GetContent(t *testing.T) {
	toolBody := "tool response"
	rawBody := "original user"
	expanded := "expanded user"
	cases := []struct {
		name     string
		message  *Message
		expected string
	}{
		{
			name:     "tool response preferred",
			message:  &Message{ToolCall: &ToolCallView{ResponsePayload: &ResponsePayloadView{InlineBody: ptr("tool response")}}},
			expected: toolBody,
		},
		{
			name:     "raw content fallback",
			message:  &Message{RawContent: ptr(rawBody), Content: ptr(expanded)},
			expected: rawBody,
		},
		{
			name:     "content used when raw missing",
			message:  &Message{Content: ptr(expanded)},
			expected: expanded,
		},
		{
			name:     "empty message returns empty string",
			message:  &Message{},
			expected: "",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.message.GetContent())
		})
	}
}

func ptr(v string) *string { return &v }
