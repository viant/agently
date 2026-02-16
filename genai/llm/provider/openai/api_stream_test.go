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

// Data-driven test: verifies stream aggregation with Responses API events.
func TestStream_ToolCalls_Aggregation(t *testing.T) {
	testCases := []struct {
		description string
		lines       []string
		expected    *llm.GenerateResponse
	}{
		{
			description: "tool_calls aggregation with finish",
			lines: []string{
				"event: response.message.tool_call.delta",
				`data: {"index":0,"delta":{"tool_call":{"id":"call_u7wc2k7fbKAxfxIJHjw3BAYF","type":"function","name":"system_exec-execute","arguments":""}}}`,
				"event: response.message.tool_call.delta",
				`data: {"index":0,"delta":{"tool_call":{"arguments":"{\""}}}`,
				"event: response.message.tool_call.delta",
				`data: {"index":0,"delta":{"tool_call":{"arguments":"commands"}}}`,
				"event: response.message.tool_call.delta",
				`data: {"index":0,"delta":{"tool_call":{"arguments":"\\":[\\""}}}`,
				"event: response.message.tool_call.delta",
				`data: {"index":0,"delta":{"tool_call":{"arguments":"date"}}}`,
				"event: response.message.tool_call.delta",
				`data: {"index":0,"delta":{"tool_call":{"arguments":" +"}}}`,
				"event: response.message.tool_call.delta",
				`data: {"index":0,"delta":{"tool_call":{"arguments":"%"}}}`,
				"event: response.message.tool_call.delta",
				`data: {"index":0,"delta":{"tool_call":{"arguments":"A"}}}`,
				"event: response.message.tool_call.delta",
				`data: {"index":0,"delta":{"tool_call":{"arguments":"\"],"}}}`,
				"event: response.message.tool_call.delta",
				`data: {"index":0,"delta":{"tool_call":{"arguments":"\""}}}`,
				"event: response.message.tool_call.delta",
				`data: {"index":0,"delta":{"tool_call":{"arguments":"timeout"}}}`,
				"event: response.message.tool_call.delta",
				`data: {"index":0,"delta":{"tool_call":{"arguments":"Ms"}}}`,
				"event: response.message.tool_call.delta",
				`data: {"index":0,"delta":{"tool_call":{"arguments":"\":\""}}}`,
				"event: response.message.tool_call.delta",
				`data: {"index":0,"delta":{"tool_call":{"arguments":"120"}}}`,
				"event: response.message.tool_call.delta",
				`data: {"index":0,"delta":{"tool_call":{"arguments":"000"}}}`,
				"event: response.message.tool_call.delta",
				`data: {"index":0,"delta":{"tool_call":{"arguments":"}"}}}`,
				// Final response object
				"event: response.completed",
				`data: {"id":"resp_1","status":"completed","model":"o4-mini-2025-04-16","output":[{"type":"message","role":"assistant","content":[],"tool_calls":[{"id":"call_u7wc2k7fbKAxfxIJHjw3BAYF","type":"function","function":{"name":"system_exec-execute","arguments":"{\"commands\":[\"date +%A\"],\"timeoutMs\":120000}"}}]}],"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}`,
			},
			expected: &llm.GenerateResponse{
				Choices: []llm.Choice{
					{
						Index:        0,
						FinishReason: "",
						Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
							{
								ID:        "call_u7wc2k7fbKAxfxIJHjw3BAYF",
								Name:      "system_exec-execute",
								Arguments: map[string]interface{}{"commands": []interface{}{"date +%A"}, "timeoutMs": float64(120000)},
								Type:      "function",
								Function:  llm.FunctionCall{Name: "system_exec-execute", Arguments: `{"commands":["date +%A"],"timeoutMs":120000}`},
							},
						}},
					},
				},
				Usage:      &llm.Usage{},
				Model:      "o4-mini-2025-04-16",
				ResponseID: "resp_1",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			body := strings.Join(tc.lines, "\n")
			srv := newLocalServerOrSkip(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/responses" {
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

func TestStream_ResponseFailed_ErrorMessage(t *testing.T) {
	lines := []string{
		"event: response.created",
		`data: {"response":{"id":"resp_1"}}`,
		"event: response.failed",
		`data: {"type":"response.failed","response":{"id":"resp_1","status":"failed","error":{"code":"model_not_found","message":"The model \\\"gpt-5.3-codex\\\" does not exist or you do not have access to it."}}}`,
	}
	body := strings.Join(lines, "\n")
	srv := newLocalServerOrSkip(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
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

	req := &llm.GenerateRequest{Messages: []llm.Message{llm.NewUserMessage("hi")}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ch, err := c.Stream(ctx, req)
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}

	var gotErr error
	for ev := range ch {
		if ev.Err != nil {
			gotErr = ev.Err
			break
		}
	}
	if assert.Error(t, gotErr) {
		assert.Contains(t, gotErr.Error(), "gpt-5.3-codex")
	}
}

func TestStream_EventError_Fallback(t *testing.T) {
	lines := []string{
		"event: error",
		`data: {"type":"error","error":{"code":"model_not_found","message":"The model \\\"gpt-5.3-codex\\\" does not exist or you do not have access to it."}}`,
	}
	body := strings.Join(lines, "\n")
	srv := newLocalServerOrSkip(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
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

	req := &llm.GenerateRequest{Messages: []llm.Message{llm.NewUserMessage("hi")}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ch, err := c.Stream(ctx, req)
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}

	var gotErr error
	for ev := range ch {
		if ev.Err != nil {
			gotErr = ev.Err
			break
		}
	}
	if assert.Error(t, gotErr) {
		assert.Contains(t, gotErr.Error(), "gpt-5.3-codex")
	}
}

func TestStream_NonSSE_JSONError_TopLevel(t *testing.T) {
	body := `{"error":{"code":"model_not_found","message":"The model \"gpt-5.3-codex\" does not exist or you do not have access to it.","type":"invalid_request_error"}}`
	srv := newLocalServerOrSkip(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, body)
	}))
	defer srv.Close()

	c := &Client{APIKey: "test"}
	c.BaseURL = srv.URL
	c.HTTPClient = srv.Client()
	c.Model = "o4-mini-2025-04-16"

	req := &llm.GenerateRequest{Messages: []llm.Message{llm.NewUserMessage("hi")}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ch, err := c.Stream(ctx, req)
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}

	var gotErr error
	for ev := range ch {
		if ev.Err != nil {
			gotErr = ev.Err
			break
		}
	}
	if assert.Error(t, gotErr) {
		assert.Contains(t, gotErr.Error(), "gpt-5.3-codex")
	}
}

func TestStream_NonSSE_JSONError_ResponseWrapped(t *testing.T) {
	body := `{"response":{"error":{"code":"model_not_found","message":"The model \"gpt-5.3-codex\" does not exist or you do not have access to it.","type":"invalid_request_error"}}}`
	srv := newLocalServerOrSkip(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, body)
	}))
	defer srv.Close()

	c := &Client{APIKey: "test"}
	c.BaseURL = srv.URL
	c.HTTPClient = srv.Client()
	c.Model = "o4-mini-2025-04-16"

	req := &llm.GenerateRequest{Messages: []llm.Message{llm.NewUserMessage("hi")}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ch, err := c.Stream(ctx, req)
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}

	var gotErr error
	for ev := range ch {
		if ev.Err != nil {
			gotErr = ev.Err
			break
		}
	}
	if assert.Error(t, gotErr) {
		assert.Contains(t, gotErr.Error(), "gpt-5.3-codex")
	}
}

func TestStream_NonSSE_ContinuationError(t *testing.T) {
	body := `{"error":{"code":"invalid_request_error","message":"previous_response_id is invalid"}}`
	srv := newLocalServerOrSkip(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, body)
	}))
	defer srv.Close()

	c := &Client{APIKey: "test"}
	c.BaseURL = srv.URL
	c.HTTPClient = srv.Client()
	c.Model = "o4-mini-2025-04-16"

	req := &llm.GenerateRequest{Messages: []llm.Message{llm.NewUserMessage("hi")}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ch, err := c.Stream(ctx, req)
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}

	var gotErr error
	for ev := range ch {
		if ev.Err != nil {
			gotErr = ev.Err
			break
		}
	}
	if assert.Error(t, gotErr) {
		assert.Contains(t, gotErr.Error(), "continuation")
	}
}

// Data-driven test: usage in final completed event should be captured.
func TestStream_UsageOnlyFinalChunk_NoEmptyChoicesEmission(t *testing.T) {
	testCases := []struct {
		description string
		lines       []string
		expected    *llm.GenerateResponse
	}{
		{
			description: "accumulate content; completed with usage",
			lines: []string{
				// content deltas
				"event: response.output_text.delta",
				`data: {"delta":"Hello"}`,
				"event: response.output_text.delta",
				`data: {"delta":" world"}`,
				// final
				"event: response.completed",
				`data: {"id":"resp_2","status":"completed","model":"o4-mini-2025-04-16","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hello world"}]}],"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}`,
			},
			expected: &llm.GenerateResponse{Choices: []llm.Choice{{
				Index:        0,
				FinishReason: "",
				Message:      llm.Message{Role: llm.RoleAssistant, Content: "Hello world"},
			}}, Usage: &llm.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}, Model: "o4-mini-2025-04-16", ResponseID: "resp_2"},
		},
		{
			description: "assistant text via multiple deltas and final completed",
			lines: []string{
				"event: response.output_text.delta",
				`data: {"delta":"Part1-"}`,
				"event: response.output_text.delta",
				`data: {"delta":"Part2"}`,
				"event: response.completed",
				`data: {"id":"resp_3","status":"completed","model":"o4-mini-2025-04-16","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Part1-Part2"}]}],"usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}`,
			},
			expected: &llm.GenerateResponse{Choices: []llm.Choice{{
				Index:        0,
				FinishReason: "",
				Message:      llm.Message{Role: llm.RoleAssistant, Content: "Part1-Part2"},
			}}, Usage: &llm.Usage{PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3}, Model: "o4-mini-2025-04-16", ResponseID: "resp_3"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			body := strings.Join(tc.lines, "\n")
			srv := newLocalServerOrSkip(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/responses" {
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

			req := &llm.GenerateRequest{Messages: []llm.Message{llm.NewUserMessage("say hi")}}
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

// newLocalServerOrSkip attempts to start an httptest.Server and skips the test
// when the environment does not permit binding a local TCP listener.
func newLocalServerOrSkip(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Skipf("skipping test: unable to start local HTTP server: %v", r)
		}
	}()
	return httptest.NewServer(handler)
}
