//go:build integration
// +build integration

package ollama

import (
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/genai/llm"
	"testing"
)

func TestNewClient(t *testing.T) {
	testCases := []struct {
		description string
		model       string
		options     []ClientOption
		expected    *Client
		request     *llm.GenerateRequest
		expectError bool
	}{
		{
			description: "client with deepseek-r1:32b model",
			model:       "deepseek-r1:32b",
			options: []ClientOption{
				WithBaseURL("http://localhost:11434"),
				WithTimeout(120),
			},
			request: &llm.GenerateRequest{
				Messages: []llm.Message{
					llm.NewSystemMessage("You are a helpful assistant."),
					llm.NewUserMessage("Why is the sky blue?"),
				},
				Options: &llm.Options{
					Temperature: 0.8,
					TopP:        0.9,
					MaxTokens:   200,
					StopWords:   []string{"Human:", "User:"},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			client, err := NewClient(context.Background(), tc.model, tc.options...)
			if !assert.Nil(t, err) {
				return
			}

			// Test pulling the model
			t.Logf("Pulling model: %s", tc.model)
			pullResp, err := client.PullModel(context.Background(), tc.model)
			if err != nil {
				t.Logf("Warning: Failed to pull model: %v", err)
				// Continue with the test even if pull fails
			} else {
				t.Logf("Pull response: %+v", pullResp)
			}

			// Test sending a chat request
			t.Logf("Sending chat request with model: %s", tc.model)
			response, err := client.Generate(context.Background(), tc.request)
			if tc.expectError {
				assert.Error(t, err)
			} else {
				if err != nil {
					t.Logf("Error sending chat request: %v", err)
					t.Fail()
					return
				}
				assert.NoError(t, err)
				assert.NotNil(t, response)
				if len(response.Choices) > 0 {
					assert.NotEmpty(t, response.Choices)
					assert.NotEmpty(t, response.Choices[0].Message.Content)
					// Log the response for debugging
					t.Logf("Response: %s", response.Choices[0].Message.Content)
				}

				// Verify token usage information
				assert.NotNil(t, response.Usage, "Usage should not be nil")
				if response.Usage != nil {
					assert.GreaterOrEqual(t, response.Usage.PromptTokens, 0, "PromptTokens should be >= 0")
					assert.GreaterOrEqual(t, response.Usage.CompletionTokens, 0, "CompletionTokens should be >= 0")
					assert.GreaterOrEqual(t, response.Usage.TotalTokens, 0, "TotalTokens should be >= 0")

					// Check if ContextTokens is populated
					t.Logf("ContextTokens length: %d", len(response.Usage.ContextTokens))
					if len(response.Usage.ContextTokens) > 0 {
						// Get the first few tokens (up to 5)
						numTokens := 5
						if len(response.Usage.ContextTokens) < numTokens {
							numTokens = len(response.Usage.ContextTokens)
						}
						t.Logf("First few context tokens: %v", response.Usage.ContextTokens[:numTokens])
					}
				}
			}
		})
	}
}
