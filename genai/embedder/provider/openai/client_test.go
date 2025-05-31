//go:build integration
// +build integration

package openai

import (
	"context"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestClient_Embed(t *testing.T) {
	// Skip the test if OPENAI_API_KEY is not set
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY environment variable not set")
	}

	// Create a client with the API key
	client := NewClient(apiKey, "text-embedding-3-small")

	// Call Embed with text directly
	vectors, totalTokens, err := client.Embed(context.Background(), []string{"test"})
	if err != nil {
		t.Fatalf("Error creating embedding: %v", err)
	}

	// Check the result
	if len(vectors) == 0 {
		t.Fatalf("Expected 1 embedding, got %d", len(vectors))
	}

	if len(vectors[0]) == 0 {
		t.Fatalf("Expected embedding with non-zero length, got %d", len(vectors[0]))
	}

	// Check token count
	if totalTokens == 0 {
		t.Fatalf("Expected non-zero token count, got 0")
	}
}

func TestClient_EmbedWithMock(t *testing.T) {
	// Skip the test if OPENAI_API_KEY is not set
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY environment variable not set")
	}

	// Create a client with the API key
	client := NewClient(apiKey, "text-embedding-3-small")

	// Call Embed with multiple texts
	vectors, totalTokens, err := client.Embed(context.Background(), []string{"test1", "test2"})
	if err != nil {
		t.Fatalf("Error embedding: %v", err)
	}

	// Check the result
	assert.True(t, len(vectors) > 0)

	// Check token count
	if totalTokens == 0 {
		t.Fatalf("Expected non-zero token count, got 0")
	}
}
