//go:build integration
// +build integration

package vertexai

import (
	"context"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestClient_Embed(t *testing.T) {
	// Create a client with the project ID
	client, err := NewClient(context.Background(), "viant-e2e", "textembedding-gecko@002")
	if err != nil {
		t.Fatalf("Error creating client: %v", err)
	}

	// Call Embed with text directly
	vectors, totalTokens, err := client.Embed(context.Background(), []string{"The quick brown fox jumps over the lazy dog."})
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

func TestClient_EmbedMultiple(t *testing.T) {
	// Skip the test if GCP_PROJECT_ID is not set
	projectID := os.Getenv("GCP_PROJECT_ID")
	if projectID == "" {
		t.Skip("GCP_PROJECT_ID environment variable not set")
	}

	// Create a client with the project ID
	client, err := NewClient(context.Background(), projectID, "textembedding-gecko")
	if err != nil {
		t.Fatalf("Error creating client: %v", err)
	}

	// Call Embed with multiple texts directly
	vectors, totalTokens, err := client.Embed(context.Background(), []string{
		"The quick brown fox jumps over the lazy dog.",
		"The five boxing wizards jump quickly.",
	})
	if err != nil {
		t.Fatalf("Error embedding: %v", err)
	}

	// Check the result
	assert.Equal(t, 2, len(vectors), "Expected 2 embeddings")

	// Check that each embedding has values
	for i, embedding := range vectors {
		assert.True(t, len(embedding) > 0, "Embedding %d has no values", i)
	}

	// Check token count
	if totalTokens == 0 {
		t.Fatalf("Expected non-zero token count, got 0")
	}
}
