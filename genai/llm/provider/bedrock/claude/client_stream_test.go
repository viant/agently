//go:build integration
// +build integration

package claude

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/genai/llm"
)

// TestStream verifies streaming path works (or gracefully falls back to Generate)
// against a real Bedrock Claude endpoint. Requires valid AWS credentials referenced
// by WithCredentialsURL and network access.
func TestStream(t *testing.T) {
	client, err := NewClient(context.Background(),
		"arn:aws:bedrock:us-west-2:458197927229:inference-profile/us.anthropic.claude-sonnet-4-5-20250929-v1:0",
		WithCredentialsURL("aws-e2e"),
		WithRegion("us-west-2"),
		WithAnthropicVersion("bedrock-2023-05-31"),
	)
	if !assert.NoError(t, err) {
		return
	}

	req := &llm.GenerateRequest{
		Messages: []llm.Message{
			llm.NewSystemMessage("You are a helpful assistant."),
			llm.NewUserMessage("Say hello and include the word STREAMING."),
		},
		Options: &llm.Options{
			Temperature: 0.2,
			MaxTokens:   128,
		},
	}

	// Set a deadline to prevent hanging tests
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	evCh, err := client.Stream(ctx, req)
	if !assert.NoError(t, err) {
		return
	}

	gotResponse := false
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("stream timed out: %v", ctx.Err())
		case ev, ok := <-evCh:
			if !ok {
				// channel closed
				if !gotResponse {
					t.Fatalf("stream closed without any response")
				}
				return
			}
			if ev.Err != nil {
				t.Fatalf("stream error: %v", ev.Err)
			}
			if ev.Response != nil {
				gotResponse = true
				// Basic assertions on the final aggregated response
				if len(ev.Response.Choices) > 0 {
					fmt.Println(ev.Response.Choices[0].Message.Content)
					assert.NotEmpty(t, ev.Response.Choices[0].Message.Content)
				}
				// We don't break immediately to allow additional deltas; rely on close
			}
		}
	}
}
