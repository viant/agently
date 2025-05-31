package registry

import (
	"context"
	"fmt"
	"sync"
)

// Registry is a minimal, in-memory fallback implementation for any Named type.
type Registry[T any] struct {
	mu     sync.RWMutex
	byName map[string]T
}

// Add stores a value in memory by its name.
func (d *Registry[T]) Add(name string, a T) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.byName[name] = a
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

// New creates a new Registry instance.
func New[T any]() *Registry[T] {
	return &Registry[T]{byName: make(map[string]T)}
}
