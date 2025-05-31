package openai

import (
	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/genai/llm"
	"testing"
)

func TestToRequest(t *testing.T) {
	testCases := []struct {
		description string
		input       llm.GenerateRequest
		expected    *Request
	}{
		{
			description: "basic conversion with options",
			input: llm.GenerateRequest{
				Messages: []llm.Message{
					{
						Role:    llm.RoleUser,
						Content: "Hello",
					},
				},
				Options: &llm.Options{
					Model:       "o4-mini",
					Temperature: 1,
					MaxTokens:   100,
					TopP:        0.9,
					N:           2,
				},
			},
			expected: &Request{
				Model: "o4-mini",
				// Temperature omitted because default 1
				MaxTokens: 100,
				TopP:      0.9,
				N:         2,
				Messages: []Message{
					{
						Role:    "user",
						Content: "Hello",
					},
				},
			},
		},
		{
			description: "conversion with tools",
			input: llm.GenerateRequest{
				Messages: []llm.Message{
					{
						Role:    llm.RoleUser,
						Content: "What's the weather?",
					},
				},
				Options: &llm.Options{
					Model:       "o4-mini",
					Temperature: 0.7,
					Tools: []llm.Tool{
						{
							Type: "function",
							Definition: llm.ToolDefinition{
								Name:        "get_weather",
								Description: "Get the weather",
								Parameters: map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"location": map[string]interface{}{
											"type":        "string",
											"description": "The location to get weather for",
										},
									},
								},
							},
						},
					},
					ToolChoice: llm.ToolChoice{
						Type: "auto",
					},
				},
			},
			expected: &Request{
				Model: "o4-mini",
				// Temperature omitted because o4-mini default 1.
				Messages: []Message{
					{
						Role:    "user",
						Content: "What's the weather?",
					},
				},
				Tools: []Tool{
					{
						Type: "function",
						Function: ToolDefinition{
							Name:        "get_weather",
							Description: "Get the weather",
							Parameters: map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"location": map[string]interface{}{
										"type":        "string",
										"description": "The location to get weather for",
									},
								},
							},
						},
					},
				},
				ToolChoice: "auto",
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

func TestToLLMSResponse(t *testing.T) {
	testCases := []struct {
		description string
		input       *Response
		expected    *llm.GenerateResponse
	}{
		{
			description: "basic response conversion",
			input: &Response{
				ID:      "resp-123",
				Object:  "chat.completion",
				Created: 1630000000,
				Model:   "gpt-4",
				Choices: []Choice{
					{
						Index: 0,
						Message: Message{
							Role:    "assistant",
							Content: "Hello, how can I help you?",
						},
						FinishReason: "stop",
					},
				},
				Usage: Usage{
					PromptTokens:     10,
					CompletionTokens: 8,
					TotalTokens:      18,
				},
			},
			expected: &llm.GenerateResponse{
				Choices: []llm.Choice{
					{
						Index: 0,
						Message: llm.Message{
							Role:    llm.RoleAssistant,
							Content: "Hello, how can I help you?",
						},
						FinishReason: "stop",
					},
				},
				Usage: &llm.Usage{
					PromptTokens:     10,
					CompletionTokens: 8,
					TotalTokens:      18,
				},
			},
		},
		{
			description: "response with tool calls",
			input: &Response{
				ID:      "resp-456",
				Object:  "chat.completion",
				Created: 1630000000,
				Model:   "gpt-4",
				Choices: []Choice{
					{
						Index: 0,
						Message: Message{
							Role:    "assistant",
							Content: "",
							ToolCalls: []ToolCall{
								{
									ID:   "call-123",
									Type: "function",
									Function: FunctionCall{
										Name:      "get_weather",
										Arguments: `{"location":"New York"}`,
									},
								},
							},
						},
						FinishReason: "tool_calls",
					},
				},
				Usage: Usage{
					PromptTokens:     15,
					CompletionTokens: 12,
					TotalTokens:      27,
				},
			},
			expected: &llm.GenerateResponse{
				Choices: []llm.Choice{
					{
						Index: 0,
						Message: llm.Message{
							Role:    llm.RoleAssistant,
							Content: "",
							ToolCalls: []llm.ToolCall{
								{
									ID:   "call-123",
									Type: "function",
									Function: llm.FunctionCall{
										Name:      "get_weather",
										Arguments: `{"location":"New York"}`,
									},
								},
							},
						},
						FinishReason: "tool_calls",
					},
				},
				Usage: &llm.Usage{
					PromptTokens:     15,
					CompletionTokens: 12,
					TotalTokens:      27,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			actual := ToLLMSResponse(tc.input)
			assert.EqualValues(t, tc.expected, actual)
		})
	}
}
