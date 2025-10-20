package embedder

import (
	"context"
	base "github.com/viant/agently/genai/embedder/provider/base"
)

// Finder defines an interface for accessing embedder clients by ID.
type Finder interface {
	Find(ctx context.Context, id string) (base.Embedder, error)
	Ids() []string
}
