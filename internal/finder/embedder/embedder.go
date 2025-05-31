package embedder

import (
	"context"
	"fmt"
	"sync"

	"github.com/tmc/langchaingo/embeddings"
	provider "github.com/viant/agently/genai/embedder/provider"
	"github.com/viant/agently/internal/registry"
)

// Finder caches embeddings.Embedder instances and lazily instantiates them using
// the generic provider.Factory.
type Finder struct {
	factory        *provider.Factory
	configRegistry *registry.Registry[*provider.Config]
	loader         provider.ConfigLoader
	embedders      map[string]embeddings.Embedder
	mux            sync.RWMutex
}

// New creates Finder instance.
func New(options ...Option) *Finder {
	d := &Finder{
		factory:        provider.New(),
		configRegistry: registry.New[*provider.Config](),
		embedders:      map[string]embeddings.Embedder{},
	}

	for _, opt := range options {
		opt(d)
	}
	return d
}

// Embedder returns a ready-to-use embeddings.Embedder by ID, creating and
// caching it on first request.
func (d *Finder) Find(ctx context.Context, id string) (embeddings.Embedder, error) {
	d.mux.RLock()
	if e, ok := d.embedders[id]; ok {
		d.mux.RUnlock()
		return e, nil
	}
	d.mux.RUnlock()

	d.mux.Lock()
	defer d.mux.Unlock()
	if e, ok := d.embedders[id]; ok { // double-check after locking
		return e, nil
	}
	config, err := d.configRegistry.Lookup(ctx, id)
	if err != nil {
		if d.loader != nil {
			config, err = d.loader.Load(ctx, id)
		}
		if err != nil {
			return nil, err
		}
	}

	if config == nil {
		return nil, fmt.Errorf("embedder config not found: %s", id)
	}
	client, err := d.factory.CreateEmbedder(ctx, &config.Options)
	if err != nil {
		return nil, err
	}
	embedder, err := embeddings.NewEmbedder(client)
	if err != nil {
		return nil, err
	}
	d.embedders[id] = embedder
	return embedder, nil
}
