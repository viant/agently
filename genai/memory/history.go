package memory

import (
	"context"
	"sync"
)

// HistoryStore manages conversation messages by conversation ID.
type HistoryStore struct {
	data map[string][]Message
	mux  sync.RWMutex
}

// History defines behaviour for conversation history storage.
type History interface {
	AddMessage(ctx context.Context, convID string, msg Message) error
	GetMessages(ctx context.Context, convID string) ([]Message, error)
	Retrieve(ctx context.Context, convID string, policy Policy) ([]Message, error)
}

// NewHistoryStore creates a new in-memory history store.
func NewHistoryStore() *HistoryStore {
	return &HistoryStore{
		data: make(map[string][]Message),
	}
}

// AddMessage stores a message under the given conversation ID.
func (h *HistoryStore) AddMessage(ctx context.Context, convID string, msg Message) error {
	h.mux.Lock()
	defer h.mux.Unlock()
	h.data[convID] = append(h.data[convID], msg)
	return nil
}

// GetMessages retrieves all messages for the conversation ID.
func (h *HistoryStore) GetMessages(ctx context.Context, convID string) ([]Message, error) {
	h.mux.RLock()
	defer h.mux.RUnlock()
	entries := h.data[convID]
	// Return a copy to avoid external modifications.
	copied := make([]Message, len(entries))
	copy(copied, entries)
	return copied, nil
}

// Retrieve returns messages filtered by the provided policy.
// If policy is nil, all messages are returned.
func (h *HistoryStore) Retrieve(ctx context.Context, convID string, policy Policy) ([]Message, error) {
	msgs, err := h.GetMessages(ctx, convID)
	if err != nil {
		return nil, err
	}
	if policy == nil {
		return msgs, nil
	}
	return policy.Apply(ctx, msgs)
}

// ListIDs returns all conversation IDs currently stored.
func (h *HistoryStore) ListIDs(ctx context.Context) ([]string, error) {
	h.mux.RLock()
	defer h.mux.RUnlock()
	ids := make([]string, 0, len(h.data))
	for id := range h.data {
		ids = append(ids, id)
	}
	return ids, nil
}

// Delete removes every message belonging to the supplied conversation ID.
func (h *HistoryStore) Delete(ctx context.Context, convID string) error {
	h.mux.Lock()
	defer h.mux.Unlock()
	delete(h.data, convID)
	return nil
}
