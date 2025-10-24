package tokens

import "sync"

// MemoryRefreshStore is an in-memory RefreshStore useful for tests or ephemeral sessions.
// Note: This store does not encrypt at rest; use only when policy allows non-persistent storage.
type MemoryRefreshStore struct {
    mu   sync.RWMutex
    data map[Key]Refresh
}

func NewMemoryRefreshStore() *MemoryRefreshStore {
    return &MemoryRefreshStore{data: make(map[Key]Refresh)}
}

func (m *MemoryRefreshStore) Get(k Key) (Refresh, bool, error) {
    m.mu.RLock()
    v, ok := m.data[k]
    m.mu.RUnlock()
    return v, ok, nil
}

func (m *MemoryRefreshStore) Set(k Key, r Refresh) error {
    m.mu.Lock()
    m.data[k] = r
    m.mu.Unlock()
    return nil
}

func (m *MemoryRefreshStore) Delete(k Key) error {
    m.mu.Lock()
    if v, ok := m.data[k]; ok {
        v.Token.Clear()
        delete(m.data, k)
    }
    m.mu.Unlock()
    return nil
}

