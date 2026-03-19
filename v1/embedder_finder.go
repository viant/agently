package v1

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/viant/agently-core/genai/embedder/provider"
	baseembed "github.com/viant/agently-core/genai/embedder/provider/base"
)

// embedderFinder resolves workspace embedder configs to runtime embedder clients.
// This mirrors modelFinder behavior and keeps plugin wiring in agently-app.
type embedderFinder struct {
	loader  provider.ConfigLoader
	factory *provider.Factory

	mu        sync.RWMutex
	embedders map[string]baseembed.Embedder
}

func newEmbedderFinder(loader provider.ConfigLoader) *embedderFinder {
	return &embedderFinder{
		loader:    loader,
		factory:   provider.New(),
		embedders: map[string]baseembed.Embedder{},
	}
}

func (f *embedderFinder) Find(ctx context.Context, id string) (baseembed.Embedder, error) {
	f.mu.RLock()
	if emb, ok := f.embedders[id]; ok && emb != nil {
		f.mu.RUnlock()
		return emb, nil
	}
	f.mu.RUnlock()

	cfg, err := f.loader.Load(ctx, id)
	if err != nil {
		fallback := filepath.ToSlash(filepath.Join("embedders", strings.TrimSpace(id)))
		cfg, err = f.loader.Load(ctx, fallback)
		if err != nil {
			return nil, err
		}
	}
	if cfg == nil {
		return nil, fmt.Errorf("embedder config not found: %s", id)
	}
	emb, err := f.factory.CreateEmbedder(ctx, &cfg.Options)
	if err != nil {
		return nil, err
	}

	f.mu.Lock()
	f.embedders[id] = emb
	f.mu.Unlock()
	return emb, nil
}

func (f *embedderFinder) Ids() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]string, 0, len(f.embedders))
	for id := range f.embedders {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
