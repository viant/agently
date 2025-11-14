package grok

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/genai/llm"
)

// roundTripFunc allows using a function as an HTTP RoundTripper.
type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestGenerate_UsageListener(t *testing.T) {
	testCases := []struct {
		name          string
		respBody      string
		expectedModel string
		expectedUsage llm.Usage
	}{
		{
			name:          "basic usage",
			respBody:      `{"id":"id","object":"chat.completion","created":0,"model":"grok-4","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":6,"total_tokens":11}}`,
			expectedModel: "grok-4",
			expectedUsage: llm.Usage{PromptTokens: 5, CompletionTokens: 6, TotalTokens: 11},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var called bool
			client := NewClient(
				"apiKey",
				tc.expectedModel,
				WithUsageListener(func(model string, usage *llm.Usage) {
					called = true
					assert.EqualValues(t, tc.expectedModel, model)
					assert.EqualValues(t, &tc.expectedUsage, usage)
				}),
			)
			client.BaseURL = "http://localhost"
			client.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(tc.respBody)), Header: make(http.Header)}, nil
			})}

			resp, err := client.Generate(context.Background(), &llm.GenerateRequest{})
			assert.NoError(t, err)
			assert.True(t, called, "usage listener should be called")
			assert.NotNil(t, resp.Usage)
			assert.EqualValues(t, tc.expectedUsage, *resp.Usage)
		})
	}
}
