package embedder

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	provider "github.com/viant/agently/genai/embedder/provider"
	baseembed "github.com/viant/agently/genai/embedder/provider/base"
	"github.com/viant/agently/internal/registry"
)

// Finder caches embedder client instances and lazily instantiates them using
// the generic provider.Factory.
type Finder struct {
	factory        *provider.Factory
	configRegistry *registry.Registry[*provider.Config]
	loader         provider.ConfigLoader
	embedders      map[string]baseembed.Embedder
	mux            sync.RWMutex
	version        int64
}

// New creates Finder instance.
func New(options ...Option) *Finder {
	d := &Finder{
		factory:        provider.New(),
		configRegistry: registry.New[*provider.Config](),
		embedders:      map[string]baseembed.Embedder{},
	}

	for _, opt := range options {
		opt(d)
	}
	return d
}

// Remove deletes an embedder entry and its config.
func (d *Finder) Remove(name string) {
	d.mux.Lock()
	if _, ok := d.embedders[name]; ok {
		delete(d.embedders, name)
	}
	d.mux.Unlock()

	d.configRegistry.Remove(name)
	atomic.AddInt64(&d.version, 1)
}

// AddConfig registers or overwrites an embedder configuration and bumps the
// internal version.
func (d *Finder) AddConfig(name string, cfg *provider.Config) {
	if cfg == nil || name == "" {
		return
	}
	d.configRegistry.Add(name, cfg)
	// Remove any instantiated embedder so next Find rebuilds it.
	d.mux.Lock()
	delete(d.embedders, name)
	d.mux.Unlock()
	atomic.AddInt64(&d.version, 1)
}

// Version returns counter changed on Add/Remove.
func (d *Finder) Version() int64 { return atomic.LoadInt64(&d.version) }

func (d *Finder) Ids() []string {
	d.mux.RLock()
	defer d.mux.RUnlock()
	ids := make([]string, 0, len(d.embedders))
	for id := range d.embedders {
		ids = append(ids, id)
	}
	return ids
}

// Find returns a ready-to-use embedder client by ID, creating and
// caching it on first request.
func (d *Finder) Find(ctx context.Context, id string) (baseembed.Embedder, error) {
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
	d.embedders[id] = client
	return client, nil
}
