package memory

import (
	"context"
	"sync"
	"time"
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

	// UpdateMessage finds message by id within convID and applies mutator.
	UpdateMessage(ctx context.Context, convID string, id string, mutate func(*Message)) error

	LatestMessage(ctx context.Context) (convID string, msg *Message, err error)
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
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}
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

// UpdateMessage applies a mutator function to the message with the given ID
// inside the specified conversation. If the message is not found the call is
// a no-op and returns nil so that callers do not have to care about races
// between polling and updates.
func (h *HistoryStore) UpdateMessage(ctx context.Context, convID string, id string, mutate func(*Message)) error {
	if mutate == nil {
		return nil
	}
	h.mux.Lock()
	defer h.mux.Unlock()
	msgs := h.data[convID]
	for i := range msgs {
		if msgs[i].ID == id {
			mutate(&msgs[i])
			break
		}
	}
	return nil
}

// LatestToolMessage scans all conversations and returns the latest tool
// message encountered. For the simple in-memory store we assume messages are
// appended in chronological order, therefore the last conversation inspected
// with a matching message provides the overall latest. While not perfectly
// accurate in concurrent scenarios it is good enough for local/CLI usage.
func (h *HistoryStore) LatestMessage(ctx context.Context) (string, *Message, error) {
	h.mux.RLock()
	defer h.mux.RUnlock()

	var latestConv string
	var latestMsg *Message
	var latestTime time.Time
	for convID, msgs := range h.data {
		for i := len(msgs) - 1; i >= 0; i-- {
			m := msgs[i]
			if latestMsg == nil || msgs[i].CreatedAt.After(latestTime) {
				latestConv = convID
				tmp := m
				latestMsg = &tmp
				latestTime = m.CreatedAt
			}
		}
	}
	return latestConv, latestMsg, nil
}
