package openai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/genai/llm"
)

// Data-driven test: verifies stream aggregation and finish-only emission.
func TestStream_ToolCalls_Aggregation(t *testing.T) {
	testCases := []struct {
		description string
		lines       []string
		expected    *llm.GenerateResponse
	}{
		{
			description: "tool_calls aggregation with finish",
			lines: []string{
				`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1755193426,"model":"o4-mini-2025-04-16","choices":[{"index":0,"delta":{"role":"assistant","content":null,"tool_calls":[{"index":0,"id":"call_u7wc2k7fbKAxfxIJHjw3BAYF","type":"function","function":{"name":"system_exec-execute","arguments":""}}]},"finish_reason":null}]}`,
				`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1755193426,"model":"o4-mini-2025-04-16","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\""}}]},"finish_reason":null}]}`,
				`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1755193426,"model":"o4-mini-2025-04-16","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"commands"}}]},"finish_reason":null}]}`,
				`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1755193426,"model":"o4-mini-2025-04-16","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\\":[\\""}}]},"finish_reason":null}]}`,
				`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1755193426,"model":"o4-mini-2025-04-16","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"date"}}]},"finish_reason":null}]}`,
				`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1755193426,"model":"o4-mini-2025-04-16","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":" +"}}]},"finish_reason":null}]}`,
				`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1755193426,"model":"o4-mini-2025-04-16","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"%"}}]},"finish_reason":null}]}`,
				`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1755193426,"model":"o4-mini-2025-04-16","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"A"}}]},"finish_reason":null}]}`,
				`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1755193426,"model":"o4-mini-2025-04-16","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"],"}}]},"finish_reason":null}]}`,
				`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1755193426,"model":"o4-mini-2025-04-16","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\""}}]},"finish_reason":null}]}`,
				`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1755193426,"model":"o4-mini-2025-04-16","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"timeout"}}]},"finish_reason":null}]}`,
				`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1755193426,"model":"o4-mini-2025-04-16","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"Ms"}}]},"finish_reason":null}]}`,
				`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1755193426,"model":"o4-mini-2025-04-16","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\":\""}}]},"finish_reason":null}]}`,
				`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1755193426,"model":"o4-mini-2025-04-16","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"120"}}]},"finish_reason":null}]}`,
				`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1755193426,"model":"o4-mini-2025-04-16","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"000"}}]},"finish_reason":null}]}`,
				`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1755193426,"model":"o4-mini-2025-04-16","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"}"}}]},"finish_reason":null}]}`,
				`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1755193426,"model":"o4-mini-2025-04-16","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
				`data: [DONE]`,
			},
			expected: &llm.GenerateResponse{Choices: []llm.Choice{{
				Index:        0,
				FinishReason: "tool_calls",
				Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{
					ID:        "call_u7wc2k7fbKAxfxIJHjw3BAYF",
					Name:      "system_exec-execute",
					Arguments: map[string]interface{}{"commands": []interface{}{"date +%A"}, "timeoutMs": float64(120000)},
					Type:      "function",
					Function:  llm.FunctionCall{Name: "system_exec-execute", Arguments: `{"commands":["date +%A"],"timeoutMs":120000}`},
				}}},
			}}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			body := strings.Join(tc.lines, "\n")
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/chat/completions" {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprint(w, body)
			}))
			defer srv.Close()

			c := &Client{APIKey: "test"}
			c.BaseURL = srv.URL
			c.HTTPClient = srv.Client()
			c.Model = "o4-mini-2025-04-16"

			req := &llm.GenerateRequest{Messages: []llm.Message{llm.NewUserMessage("run a command")}}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			ch, err := c.Stream(ctx, req)
			if err != nil {
				t.Fatalf("Stream error: %v", err)
			}

			var actual *llm.GenerateResponse
			for ev := range ch {
				if ev.Err != nil {
					t.Fatalf("streaming error: %v", ev.Err)
				}
				actual = ev.Response
			}
			assert.EqualValues(t, tc.expected, actual)
		})
	}
}
