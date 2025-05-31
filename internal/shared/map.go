package shared

import "sync"

type Map[K comparable, V any] struct {
	m   map[K]V
	mux sync.RWMutex
}

func (m *Map[K, V]) Get(key K) (V, bool) {
	m.mux.RLock()
	defer m.mux.RUnlock()
	value, ok := m.m[key]
	return value, ok
}

func (m *Map[K, V]) Set(key K, value V) {
	m.mux.Lock()
	defer m.mux.Unlock()
	m.m[key] = value
}

func (m *Map[K, V]) Delete(key K) {
	m.mux.Lock()
	defer m.mux.Unlock()
	delete(m.m, key)
}

func (m *Map[K, V]) Len() int {
	m.mux.RLock()
	defer m.mux.RUnlock()
	return len(m.m)
}

func (m *Map[K, V]) Exists(key K) bool {
	m.mux.RLock()
	defer m.mux.RUnlock()
	_, ok := m.m[key]
	return ok
}

func (m *Map[K, V]) Keys() []K {
	m.mux.RLock()
	defer m.mux.RUnlock()
	keys := make([]K, 0, len(m.m))
	for k := range m.m {
		keys = append(keys, k)
	}
	return keys
}

func (m *Map[K, V]) Map() map[K]V {
	m.mux.RLock()
	defer m.mux.RUnlock()
	clone := make(map[K]V, len(m.m))
	for k, v := range m.m {
		clone[k] = v
	}
	return clone
}

func (m *Map[K, V]) Clone() Map[K, V] {
	m.mux.RLock()
	defer m.mux.RUnlock()
	clone := make(map[K]V, len(m.m))
	for k, v := range m.m {
		clone[k] = v
	}
	return Map[K, V]{
		m: clone,
	}
}

func (m *Map[K, V]) Range(f func(key K, value V) bool) {
	m.mux.RLock()
	defer m.mux.RUnlock()
	for k, v := range m.m {
		if !f(k, v) {
			break
		}
	}
}

func NewMap[K comparable, V any]() Map[K, V] {
	return Map[K, V]{
		m: make(map[K]V),
	}
}
