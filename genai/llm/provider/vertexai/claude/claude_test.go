//go:build integration
// +build integration

package claude

import (
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/genai/llm"
	"testing"
)

// max is a helper function to return the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func TestNewClient(t *testing.T) {
	testCases := []struct {
		description string
		projectID   string
		model       string
		options     []ClientOption
		expected    *Client
		request     *llm.GenerateRequest
		expectError bool
	}{

		{
			description: "client with custom anthropic version",
			projectID:   "viant-e2e",
			model:       "claude-3-7-sonnet@20250219",
			options: []ClientOption{
				WithLocation("us-east5"),
				WithProjectID("viant-e2e"),
				WithAnthropicVersion("vertex-2023-10-16")},

			request: &llm.GenerateRequest{
				Messages: []llm.Message{
					llm.NewSystemMessage("You are a helpful assistant."),
					llm.NewUserMessage("Hello, how are you?"),
				},
				Options: &llm.Options{
					Temperature: 0.7,
					MaxTokens:   50,
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
			response, err := client.Generate(context.Background(), tc.request)
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, response)
				if len(response.Choices) > 0 {
					assert.NotEmpty(t, response.Choices)
					assert.NotEmpty(t, response.Choices[0].Message.Content)
					assert.NotNil(t, response.Usage)
					assert.Greater(t, response.Usage.TotalTokens, 0)
					// Log the response for debugging
					t.Logf("Response: %s", response.Choices[0].Message.Content)
				}
			}
		})
	}
}
