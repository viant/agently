package openai

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/memory"
)

// roundTripFunc allows using a function as an HTTP RoundTripper.
type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestGenerate_UsageListener(t *testing.T) {
	testCases := []struct {
		name          string
		respBody      string
		expectedModel string
		expectedUsage llm.Usage
	}{
		{
			name:          "basic usage",
			respBody:      `{"id":"id","object":"chat.completion","created":0,"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":""}],"usage":{"prompt_tokens":5,"completion_tokens":6,"total_tokens":11}}`,
			expectedModel: "test-model",
			expectedUsage: llm.Usage{PromptTokens: 5, CompletionTokens: 6, TotalTokens: 11},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var called bool
			enabled := false
			client := NewClient(
				"apiKey",
				tc.expectedModel,
				WithUsageListener(func(model string, usage *llm.Usage) {
					called = true
					assert.EqualValues(t, tc.expectedModel, model)
					assert.EqualValues(t, &tc.expectedUsage, usage)
				}),
				// Force legacy chat/completions path for this unit test
				WithContextContinuation(&enabled),
			)
			client.BaseURL = "http://localhost"
			client.HTTPClient = &http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(tc.respBody)),
						Header:     make(http.Header),
					}, nil
				}),
			}

			resp, err := client.Generate(context.Background(), &llm.GenerateRequest{})
			assert.NoError(t, err)
			assert.True(t, called, "usage listener should be called")
			assert.NotNil(t, resp.Usage)
			assert.EqualValues(t, tc.expectedUsage, *resp.Usage)
		})
	}
}

func TestCreateHTTPResponsesApiRequest_BackendSessionHeader(t *testing.T) {
	client := NewClient("api-key", "gpt-5.3-codex")
	client.BaseURL = "https://chatgpt.com/backend-api/codex"

	ctx := memory.WithConversationID(context.Background(), "conv-123")
	req, err := client.createHTTPResponsesApiRequest(ctx, []byte(`{"model":"gpt-5.3-codex","input":[]}`))
	assert.NoError(t, err)
	assert.EqualValues(t, "conv-123", req.Header.Get("session_id"))
}

func TestApplyBackendSessionDefaults_PromptCacheKeyFromConversation(t *testing.T) {
	client := NewClient("api-key", "gpt-5.3-codex")
	client.BaseURL = "https://chatgpt.com/backend-api/codex"

	req := &Request{Model: "gpt-5.3-codex"}
	ctx := memory.WithConversationID(context.Background(), "conv-xyz")
	client.applyBackendSessionDefaults(ctx, req)
	assert.EqualValues(t, "conv-xyz", req.PromptCacheKey)
}
