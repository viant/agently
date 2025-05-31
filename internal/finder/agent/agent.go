package agent

import (
	"context"
	"fmt"
	"sync"

	"github.com/viant/agently/genai/agent"
)

// ensure Finder implements the public interface
var _ agent.Finder = (*Finder)(nil)

// Finder is an in-memory cache with optional lazy-loading through Loader.
type Finder struct {
	mu     sync.RWMutex
	items  map[string]*agent.Agent
	loader agent.Loader
}

// Add stores an Agent under the provided name key.
func (d *Finder) Add(name string, a *agent.Agent) {
	if a == nil || name == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.items[name] = a
}

// Agent returns an Agent by name, loading it if not found in the cache.
func (d *Finder) Find(ctx context.Context, name string) (*agent.Agent, error) {
	d.mu.RLock()
	if a, ok := d.items[name]; ok {
		d.mu.RUnlock()
		return a, nil
	}
	d.mu.RUnlock()

	if d.loader == nil {
		return nil, fmt.Errorf("agent not found: %s", name)
	}
	a, err := d.loader.Load(ctx, name)
	if err != nil {
		return nil, err
	}
	if a != nil {
		d.mu.Lock()
		d.items[name] = a
		d.mu.Unlock()
	}
	return a, nil
}

// New creates Finder instance.
func New(options ...Option) *Finder {
	d := &Finder{items: map[string]*agent.Agent{}}
	for _, opt := range options {
		opt(d)
	}
	return d
}
