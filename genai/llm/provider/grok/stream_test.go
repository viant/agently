package grok

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

func TestStream_Grok_TextAggregation_Usage(t *testing.T) {
	lines := []string{
		`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":0,"model":"grok-4","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"}}],"usage":{"prompt_tokens":10,"completion_tokens":1,"total_tokens":11,"prompt_tokens_details":{"text_tokens":10,"audio_tokens":0,"image_tokens":0,"cached_tokens":0}}}`,
		`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":0,"model":"grok-4","choices":[{"index":0,"delta":{"content":", world"}}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12,"prompt_tokens_details":{"text_tokens":10,"audio_tokens":0,"image_tokens":0,"cached_tokens":0}}}`,
		`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":0,"model":"grok-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":3,"total_tokens":13,"prompt_tokens_details":{"text_tokens":10,"audio_tokens":0,"image_tokens":0,"cached_tokens":0}}}`,
		`data: [DONE]`,
	}
	body := strings.Join(lines, "\n")
	srv := newLocalServerOrSkip(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, body)
	}))
	defer srv.Close()

	var gotModel string
	var gotUsage *llm.Usage
	c := NewClient("apiKey", "grok-4", WithUsageListener(func(m string, u *llm.Usage) { gotModel, gotUsage = m, u }))
	c.BaseURL = srv.URL
	c.HTTPClient = srv.Client()

	req := &llm.GenerateRequest{Messages: []llm.Message{llm.NewUserMessage("hello")}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ch, err := c.Stream(ctx, req)
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}

	var last *llm.GenerateResponse
	for ev := range ch {
		if ev.Err != nil {
			t.Fatalf("streaming error: %v", ev.Err)
		}
		last = ev.Response
	}
	assert.NotNil(t, last)
	assert.Equal(t, "Hello, world", last.Choices[0].Message.Content)
	// usage listener should be called once with final cumulative
	assert.Equal(t, "grok-4", gotModel)
	assert.EqualValues(t, &llm.Usage{PromptTokens: 10, CompletionTokens: 3, TotalTokens: 13, PromptCachedTokens: 0}, gotUsage)
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
