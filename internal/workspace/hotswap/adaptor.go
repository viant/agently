package hotswap

import "context"

// LoaderFunc loads an object identified by name from an external source.
// It usually parses YAML/JSON from disk and returns a fully validated object.
type LoaderFunc[T any] func(ctx context.Context, name string) (T, error)

// SetFunc stores a value under the given name inside an in-memory registry.
// Implementations must be thread-safe.
type SetFunc[T any] func(name string, value T)

// RemoveFunc deletes a value from the registry by name. It must be safe when
// the entry does not exist.
type RemoveFunc func(name string)

// adaptor is a generic bridge that converts HotSwapManager events into calls
// on a specific loader+registry pair. It implements the Reloadable interface.
//
// Behaviour:
//   - AddOrUpdate → load(), then set(). Any loader error is returned so the
//     manager can log/report it.
//   - Delete      → remove() regardless of whether the key exists.
//
// All state is external; the adaptor itself is stateless and therefore safe
// for concurrent use by virtue of delegating concurrency to the registry
// implementation.
type adaptor[T any] struct {
	load   LoaderFunc[T]
	set    SetFunc[T]
	remove RemoveFunc
}

// NewAdaptor builds a Reloadable that delegates to the provided callbacks.
func NewAdaptor[T any](load LoaderFunc[T], set SetFunc[T], remove RemoveFunc) Reloadable {
	return &adaptor[T]{load: load, set: set, remove: remove}
}

func (a *adaptor[T]) Reload(ctx context.Context, name string, what Action) error {
	switch what {
	case AddOrUpdate:
		val, err := a.load(ctx, name)
		if err != nil {
			return err
		}
		a.set(name, val)
		return nil
	case Delete:
		a.remove(name)
		return nil
	default:
		return nil
	}
}
