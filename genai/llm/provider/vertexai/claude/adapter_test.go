//go:build integration
// +build integration

package claude

import (
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/genai/llm"
	"testing"
)

func TestToRequest(t *testing.T) {
	testCases := []struct {
		description string
		input       *llm.GenerateRequest
		expected    *Request
	}{
		{
			description: "simple chat request",
			input: &llm.GenerateRequest{
				Messages: []llm.Message{
					llm.NewSystemMessage("You are a helpful assistant."),
					llm.NewUserMessage("Hello, how are you?"),
				},
				Options: &llm.Options{
					MaxTokens: 100,
					Stream:    true,
				},
			},
			expected: &Request{
				AnthropicVersion: defaultAnthropicVersion,
				Messages: []Message{
					{
						Role: "system",
						Content: []ContentBlock{
							{
								Type: "text",
								Text: "You are a helpful assistant.",
							},
							{
								Type: "text",
								Text: "You are a helpful assistant.",
							},
						},
					},
					{
						Role: "user",
						Content: []ContentBlock{
							{
								Type: "text",
								Text: "Hello, how are you?",
							},
							{
								Type: "text",
								Text: "Hello, how are you?",
							},
						},
					},
				},
				MaxTokens: 100,
				Stream:    true,
			},
		},
		{
			description: "chat request with thinking",
			input: &llm.GenerateRequest{
				Messages: []llm.Message{
					llm.NewUserMessage("Solve this math problem: 2 + 2 = ?"),
				},
				Options: &llm.Options{
					MaxTokens: 50,
					Thinking: &llm.Thinking{
						Type:         "enabled",
						BudgetTokens: 2048,
					},
				},
			},
			expected: &Request{
				AnthropicVersion: defaultAnthropicVersion,
				Messages: []Message{
					{
						Role: "user",
						Content: []ContentBlock{
							{
								Type: "text",
								Text: "Solve this math problem: 2 + 2 = ?",
							},
							{
								Type: "text",
								Text: "Solve this math problem: 2 + 2 = ?",
							},
						},
					},
				},
				MaxTokens: 50,
				Thinking: &Thinking{
					Type:         "enabled",
					BudgetTokens: 2048,
				},
			},
		},
		{
			description: "chat request with content items",
			input: &llm.GenerateRequest{
				Messages: []llm.Message{
					{
						Role: llm.RoleUser,
						Items: []llm.ContentItem{
							{
								Type:   llm.ContentTypeText,
								Source: llm.SourceRaw,
								Data:   "What's in this image?",
							},
							{
								Type:     llm.ContentTypeImage,
								Source:   llm.SourceBase64,
								Data:     "base64encodedimage",
								MimeType: "image/jpeg",
							},
						},
					},
				},
				Options: &llm.Options{
					MaxTokens: 200,
				},
			},
			expected: &Request{
				AnthropicVersion: defaultAnthropicVersion,
				Messages: []Message{
					{
						Role: "user",
						Content: []ContentBlock{
							{
								Type: "text",
								Text: "What's in this image?",
							},
							{
								Type: "image",
								Source: &Source{
									Type:      "base64",
									MediaType: "image/jpeg",
									Data:      "base64encodedimage",
								},
							},
						},
					},
				},
				MaxTokens: 200,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			ctx := context.Background()
			actual, err := ToRequest(ctx, tc.input)
			assert.NoError(t, err)
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
			description: "message response",
			input: &Response{
				Type: "message",
				Message: Message{
					Role: "assistant",
					Content: []ContentBlock{
						{
							Type: "text",
							Text: "I'm doing well, thank you for asking!",
						},
					},
				},
			},
			expected: &llm.GenerateResponse{
				Choices: []llm.Choice{
					{
						Index: 0,
						Message: llm.Message{
							Role:    llm.RoleAssistant,
							Content: "I'm doing well, thank you for asking!",
							Items: []llm.ContentItem{
								{
									Type:   llm.ContentTypeText,
									Source: llm.SourceRaw,
									Data:   "I'm doing well, thank you for asking!",
									Text:   "I'm doing well, thank you for asking!",
								},
							},
						},
						FinishReason: "stop",
					},
				},
			},
		},
		{
			description: "delta response",
			input: &Response{
				Type: "message_delta",
				Delta: &Delta{
					Type:       "text_delta",
					Text:       "Hello",
					StopReason: "max_tokens",
				},
			},
			expected: &llm.GenerateResponse{
				Choices: []llm.Choice{
					{
						Index: 0,
						Message: llm.Message{
							Role:    llm.RoleAssistant,
							Content: "Hello",
							Items: []llm.ContentItem{
								{
									Type:   llm.ContentTypeText,
									Source: llm.SourceRaw,
									Data:   "Hello",
									Text:   "Hello",
								},
							},
						},
						FinishReason: "max_tokens",
					},
				},
			},
		},
		{
			description: "error response",
			input: &Response{
				Type: "error",
				Error: &Error{
					Type:    "invalid_request_error",
					Message: "Invalid request parameters",
				},
			},
			expected: &llm.GenerateResponse{
				Choices: []llm.Choice{
					{
						Index: 0,
						Message: llm.Message{
							Role:    llm.RoleAssistant,
							Content: "Error: Invalid request parameters",
							Items: []llm.ContentItem{
								{
									Type:   llm.ContentTypeText,
									Source: llm.SourceRaw,
									Data:   "Error: Invalid request parameters",
									Text:   "Error: Invalid request parameters",
								},
							},
						},
						FinishReason: "error",
					},
				},
			},
		},
		{
			description: "vertexai claude response",
			input: &Response{
				ID:    "msg_vrtx_01AXaTLR8UwoMgE47bFqhVhs",
				Type:  "message",
				Role:  "assistant",
				Model: "claude-3-7-sonnet-20250219",
				Content: []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": "I'm doing well, thank you for asking! I'm here and ready to help you with any questions or tasks you might have. How can I assist you today?",
					},
				},
				StopReason: "end_turn",
				Usage: &Usage{
					InputTokens:              19,
					CacheCreationInputTokens: 0,
					CacheReadInputTokens:     0,
					OutputTokens:             37,
				},
			},
			expected: &llm.GenerateResponse{
				Choices: []llm.Choice{
					{
						Index: 0,
						Message: llm.Message{
							Role:    llm.RoleAssistant,
							Content: "I'm doing well, thank you for asking! I'm here and ready to help you with any questions or tasks you might have. How can I assist you today?",
							Items: []llm.ContentItem{
								{
									Type:   llm.ContentTypeText,
									Source: llm.SourceRaw,
									Data:   "I'm doing well, thank you for asking! I'm here and ready to help you with any questions or tasks you might have. How can I assist you today?",
									Text:   "I'm doing well, thank you for asking! I'm here and ready to help you with any questions or tasks you might have. How can I assist you today?",
								},
							},
						},
						FinishReason: "end_turn",
					},
				},
				Usage: &llm.Usage{
					PromptTokens:     19,
					CompletionTokens: 37,
					TotalTokens:      56,
				},
				Model: "claude-3-7-sonnet-20250219",
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

func TestVertexAIResponseToLLMS(t *testing.T) {
	testCases := []struct {
		description string
		input       *VertexAIResponse
		expected    *llm.GenerateResponse
	}{
		{
			description: "vertexai claude response",
			input: &VertexAIResponse{
				ID:    "msg_vrtx_01AXaTLR8UwoMgE47bFqhVhs",
				Type:  "message",
				Role:  "assistant",
				Model: "claude-3-7-sonnet-20250219",
				Content: []TextContent{
					{
						Type: "text",
						Text: "I'm doing well, thank you for asking! I'm here and ready to help you with any questions or tasks you might have. How can I assist you today?",
					},
				},
				StopReason: "end_turn",
				Usage: &Usage{
					InputTokens:              19,
					CacheCreationInputTokens: 0,
					CacheReadInputTokens:     0,
					OutputTokens:             37,
				},
			},
			expected: &llm.GenerateResponse{
				Choices: []llm.Choice{
					{
						Index: 0,
						Message: llm.Message{
							Role:    llm.RoleAssistant,
							Content: "I'm doing well, thank you for asking! I'm here and ready to help you with any questions or tasks you might have. How can I assist you today?",
							Items: []llm.ContentItem{
								{
									Type:   llm.ContentTypeText,
									Source: llm.SourceRaw,
									Data:   "I'm doing well, thank you for asking! I'm here and ready to help you with any questions or tasks you might have. How can I assist you today?",
									Text:   "I'm doing well, thank you for asking! I'm here and ready to help you with any questions or tasks you might have. How can I assist you today?",
								},
							},
						},
						FinishReason: "end_turn",
					},
				},
				Usage: &llm.Usage{
					PromptTokens:     19,
					CompletionTokens: 37,
					TotalTokens:      56,
				},
				Model: "claude-3-7-sonnet-20250219",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			actual := VertexAIResponseToLLMS(tc.input)
			assert.EqualValues(t, tc.expected, actual)
		})
	}
}
