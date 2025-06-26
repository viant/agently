package registry

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

// Registry is a minimal, in-memory fallback implementation for any Named type.
type Registry[T any] struct {
	mu      sync.RWMutex
	byName  map[string]T
	version int64
}

// Add stores a value in memory by its name.
func (d *Registry[T]) Add(name string, a T) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.byName[name] = a
	atomic.AddInt64(&d.version, 1)
}

// Remove deletes a value by name from the registry. No-op when name is not
// present. Each successful removal bumps the internal version so that
// observers can detect changes.
func (d *Registry[T]) Remove(name string) {
	d.mu.Lock()
	if _, ok := d.byName[name]; ok {
		delete(d.byName, name)
		atomic.AddInt64(&d.version, 1)
	}
	d.mu.Unlock()
}

// List retrieves all values stored in memory.
func (d *Registry[T]) List(ctx context.Context) ([]T, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var items []T
	for _, item := range d.byName {
		items = append(items, item)
	}
	return items, nil
}

// Lookup retrieves a value by its name from memory.
func (d *Registry[T]) Lookup(ctx context.Context, name string) (T, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if a, ok := d.byName[name]; ok {
		return a, nil
	}
	var zero T
	return zero, fmt.Errorf("item not found: %s (memory DAO)", name)
}

// Version returns a monotonically increasing counter that changes whenever
// the registry content is modified via Add or Remove. It is safe for
// concurrent use.
func (d *Registry[T]) Version() int64 {
	return atomic.LoadInt64(&d.version)
}

// New creates a new Registry instance.
func New[T any]() *Registry[T] {
	return &Registry[T]{byName: make(map[string]T)}
}
