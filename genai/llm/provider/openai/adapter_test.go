package openai

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/genai/llm"
)

func TestToRequest_StreamFlag(t *testing.T) {
	testCases := []struct {
		description string
		input       llm.GenerateRequest
		expected    *Request
	}{
		{
			description: "streaming enabled",
			input: llm.GenerateRequest{
				Messages: []llm.Message{
					llm.NewUserMessage("Hello from user"),
				},
				Options: &llm.Options{
					Model:  "test-model",
					Stream: true,
				},
			},
			expected: &Request{
				Model:  "test-model",
				Stream: true,
				Messages: []Message{
					{
						Role:    "user",
						Content: []ContentItem{{Type: "text", Text: "Hello from user"}},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			actual := ToRequest(&tc.input)
			assert.EqualValues(t, tc.expected, actual)
		})
	}
}

// Test mapping of tool calls and tool call result ID from OpenAI response to llm.Message
func TestToLLMSResponse_ToolCallsAndToolCallId(t *testing.T) {
	// prepare a simulated OpenAI response with tool_calls and tool_call_id
	resp := &Response{
		ID:    "chatcmpl_123",
		Model: "gpt-test",
		Choices: []Choice{{
			Index: 0,
			Message: Message{
				Role:    "assistant",
				Name:    "assistant-name",
				Content: "result text",
				ToolCalls: []ToolCall{{
					ID:   "cid123",
					Type: "function",
					Function: FunctionCall{
						Name:      "doThing",
						Arguments: `{"x":1}`,
					},
				}},
				ToolCallId: "cid123",
			},
			FinishReason: "stop",
		}},
		Usage: Usage{PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5},
	}
	out := ToLLMSResponse(resp)
	assert.Equal(t, "gpt-test", out.Model)
	assert.Equal(t, "chatcmpl_123", out.ResponseID)
	assert.Len(t, out.Choices, 1)
	msg := out.Choices[0].Message
	assert.EqualValues(t, llm.RoleAssistant, msg.Role)
	assert.Equal(t, "assistant-name", msg.Name)
	assert.Equal(t, "result text", msg.Content)
	// verify tool calls mapping
	expectedCalls := []llm.ToolCall{{
		ID:   "cid123",
		Type: "function",
		Function: llm.FunctionCall{
			Name:      "doThing",
			Arguments: `{"x":1}`,
		},
	}}
	assert.EqualValues(t, expectedCalls, msg.ToolCalls)
	// verify tool call result ID mapping
	assert.Equal(t, "cid123", msg.ToolCallId)
}

// TestToRequest_ReasoningSummary ensures that reasoning.summary="auto" is propagated
// only for supported models (o3, o4-mini, codex-mini-latest).
func TestToRequest_ReasoningSummary(t *testing.T) {
	testCases := []struct {
		description string
		input       llm.GenerateRequest
		expected    *Request
	}{
		{
			description: "reasoning summary auto for supported model",
			input: llm.GenerateRequest{
				Messages: []llm.Message{llm.NewUserMessage("Hello reasoning")},
				Options:  &llm.Options{Model: "o3", Reasoning: &llm.Reasoning{Summary: "auto"}},
			},
			expected: &Request{
				Model:     "o3",
				Reasoning: &llm.Reasoning{Summary: "auto"},
				Messages:  []Message{{Role: "user", Content: []ContentItem{{Type: "text", Text: "Hello reasoning"}}}},
			},
		},
		{
			description: "reasoning summary ignored for unsupported model",
			input: llm.GenerateRequest{
				Messages: []llm.Message{llm.NewUserMessage("Ignore")},
				Options:  &llm.Options{Model: "test-model", Reasoning: &llm.Reasoning{Summary: "auto"}},
			},
			expected: &Request{
				Model:    "test-model",
				Messages: []Message{{Role: "user", Content: []ContentItem{{Type: "text", Text: "Ignore"}}}},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			actual := ToRequest(&tc.input)
			assert.EqualValues(t, tc.expected, actual)
		})
	}
}
