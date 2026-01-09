package agent

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/viant/agently/genai/agent"
)

// ensure Finder implements the public interface
var _ agent.Finder = (*Finder)(nil)

// Finder is an in-memory cache with optional lazy-loading through Loader.
type Finder struct {
	mu      sync.RWMutex
	items   map[string]*agent.Agent
	loader  agent.Loader
	version int64
}

// All returns a snapshot of all cached agents.
// It does not invoke the loader.
func (d *Finder) All() []*agent.Agent {
	if d == nil {
		return nil
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if len(d.items) == 0 {
		return nil
	}
	out := make([]*agent.Agent, 0, len(d.items))
	for _, a := range d.items {
		if a == nil {
			continue
		}
		out = append(out, a)
	}
	return out
}

// Add stores an Agent under the provided name key.
func (d *Finder) Add(name string, a *agent.Agent) {
	if a == nil || name == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.items[name] = a
	atomic.AddInt64(&d.version, 1)
}

// Remove deletes cached agent by name and bumps Version.
func (d *Finder) Remove(name string) {
	d.mu.Lock()
	if _, ok := d.items[name]; ok {
		delete(d.items, name)
		atomic.AddInt64(&d.version, 1)
	}
	d.mu.Unlock()
}

// Version returns internal version counter.
func (d *Finder) Version() int64 {
	return atomic.LoadInt64(&d.version)
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
