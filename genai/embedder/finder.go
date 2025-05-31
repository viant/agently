package embedder

import (
	"context"
	"github.com/tmc/langchaingo/embeddings"
)

// Finder finder  defines an interface for accessing embeddings.Embedder instances by ID.
type Finder interface {
	Find(ctx context.Context, id string) (embeddings.Embedder, error)
}
