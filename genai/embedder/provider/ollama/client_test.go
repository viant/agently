//go:build integration
// +build integration

package ollama

import (
	"context"
	"testing"
)

func TestClient_Embed(t *testing.T) {
	// Create a client with the default model
	client := NewClient(defaultEmbeddingModel)
	// Call Embed
	embeddings, _, err := client.Embed(context.Background(), []string{"The sky is blue because of Rayleigh scattering"})
	if err != nil {
		t.Fatalf("Error creating embedding: %v", err)
	}

	// Check the result
	if len(embeddings) == 0 {
		t.Fatalf("Expected at least 1 embedding, got %d", len(embeddings))
	}
}
