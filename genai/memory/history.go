package memory

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"sync"
	"time"
)

// HistoryStore manages conversation messages by conversation ID.
type HistoryStore struct {
	data map[string][]Message
	mux  sync.RWMutex

	meta map[string]ConversationMeta // key: convID
}

// History defines behaviour for conversation history storage.
type History interface {
	AddMessage(ctx context.Context, msg Message) error
	GetMessages(ctx context.Context, convID string) ([]Message, error)
	Retrieve(ctx context.Context, convID string, policy Policy) ([]Message, error)

	// UpdateMessage finds message by id within convID and applies mutator.
	UpdateMessage(ctx context.Context, messageId string, mutate func(*Message)) error

	// LookupMessage searches all conversations and returns the first message
	LookupMessage(ctx context.Context, messageID string) (*Message, error)

	LatestMessage(ctx context.Context) (msg *Message, err error)

	// --- Conversation meta management ------------------------------
	CreateMeta(ctx context.Context, id, parentID, title, visibility string)
	Meta(ctx context.Context, id string) (*ConversationMeta, bool)
	Children(ctx context.Context, parentID string) ([]ConversationMeta, bool)
}

// NewHistoryStore creates a new in-memory history store.
func NewHistoryStore() *HistoryStore {
	return &HistoryStore{
		data: make(map[string][]Message),
		meta: make(map[string]ConversationMeta),
	}
}

// AddMessage stores a message under the given conversation ID.
func (h *HistoryStore) AddMessage(ctx context.Context, msg Message) error {
	h.mux.Lock()
	defer h.mux.Unlock()
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}
	convID := msg.ConversationID

	// Prevent duplicate messages with the same ID within a single conversation.
	// While upstream components should ideally never emit duplicates, we add
	// this guard to keep the history store resilient against accidentally
	// repeated events (e.g. when multiple event aliases are published).
	if msg.ID != "" {
		for _, existing := range h.data[convID] {
			if existing.ID == msg.ID {
				// Message already exists – do not add another copy.
				return nil
			}
		}
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

// MessagesSince returns the slice of messages beginning with the one whose
// ID matches sinceID (inclusive) followed by every subsequent message in
// chronological order. If sinceID is empty the full history is returned. When
// the supplied ID cannot be found the returned slice is empty and no error is
// raised so that callers can treat it identical to "not yet available".
func (h *HistoryStore) MessagesSince(ctx context.Context, convID string, sinceID string) ([]Message, error) {
	if sinceID == "" {
		return h.GetMessages(ctx, convID)
	}

	all, err := h.GetMessages(ctx, convID)
	if err != nil {
		return nil, err
	}

	start := -1
	for i, m := range all {
		if m.ID == sinceID {
			start = i
			break
		}
	}
	if start == -1 {
		return []Message{}, nil
	}
	return all[start:], nil
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

// EnsureConversation makes sure a conversation key exists even if no
// messages have been added yet. It is safe to call concurrently and will not
// overwrite existing entries. The method is useful for adapters that need to
// create a conversation before the first user message arrives (e.g. when the
// UI offers a "new chat" button).
func (h *HistoryStore) EnsureConversation(convID string) {
	if convID == "" {
		return
	}
	h.mux.Lock()
	defer h.mux.Unlock()
	if _, exists := h.data[convID]; !exists {
		h.data[convID] = []Message{}
	}
}

// UpdateMessage applies a mutator function to the message with the given ID
// across all conversations. If the message is not found the call is
// a no-op and returns nil so that callers do not have to care about races
// between polling and updates.
func (h *HistoryStore) UpdateMessage(ctx context.Context, messageId string, mutate func(*Message)) error {
	if mutate == nil {
		return nil
	}
	h.mux.Lock()
	defer h.mux.Unlock()
	for _, msgs := range h.data {
		for i := range msgs {
			if msgs[i].ID == messageId {
				mutate(&msgs[i])
				return nil
			}
		}
	}
	return fmt.Errorf("message %s not found", messageId)
}

// LookupMessage implements History.LookupMessage by scanning all conversations
// for the first message whose ID matches messageID. As the in-memory store
// keeps each conversation in a simple slice, a linear scan is acceptable for
// local usage. The returned Message is a copy so callers cannot mutate the
// store inadvertently.
func (h *HistoryStore) LookupMessage(ctx context.Context, messageID string) (*Message, error) {
	if messageID == "" {
		return nil, nil
	}
	h.mux.RLock()
	defer h.mux.RUnlock()
	for _, msgs := range h.data {
		for _, m := range msgs {
			if m.ID == messageID {
				copy := m
				return &copy, nil
			}
		}
	}
	return nil, nil
}

// LatestMessage scans all conversations and returns the latest message encountered.
// For the simple in-memory store we assume messages are appended in chronological order,
// therefore the last conversation inspected with a matching message provides the overall latest.
// While not perfectly accurate in concurrent scenarios it is good enough for local/CLI usage.
func (h *HistoryStore) LatestMessage(ctx context.Context) (*Message, error) {
	h.mux.RLock()
	defer h.mux.RUnlock()

	var latestMsg *Message
	var latestTime time.Time
	for convID, msgs := range h.data {
		for i := len(msgs) - 1; i >= 0; i-- {
			m := msgs[i]
			if latestMsg == nil || msgs[i].CreatedAt.After(latestTime) {
				tmp := m
				tmp.ConversationID = convID
				latestMsg = &tmp
				latestTime = m.CreatedAt
			}
		}
	}
	return latestMsg, nil
}

// NewChildConversation creates a conversation ID, stores meta and returns it.
// It is safe to call with nil History.
func NewChildConversation(ctx context.Context, hist History, parentID, title, visibility string) string {
	id := uuid.NewString()
	if hist == nil {
		return id
	}
	hist.CreateMeta(ctx, id, parentID, title, visibility)
	// also ensure conversation key exists so messages can be added.
	if hs, ok := hist.(*HistoryStore); ok {
		hs.EnsureConversation(id)
	}
	return id
}

// -------------------------------------------------------------------
// Conversation meta operations
// -------------------------------------------------------------------

// CreateMeta registers metadata for a conversation id. If meta already exists
// it is left unchanged.
func (h *HistoryStore) CreateMeta(ctx context.Context, id, parentID, title, visibility string) {
	if id == "" {
		return
	}
	h.mux.Lock()
	defer h.mux.Unlock()
	if _, exists := h.meta[id]; exists {
		return
	}
	h.meta[id] = ConversationMeta{
		ID:         id,
		ParentID:   parentID,
		Title:      title,
		Visibility: visibility,
		CreatedAt:  time.Now(),
	}
}

// Meta fetches conversation meta by id.
func (h *HistoryStore) Meta(ctx context.Context, id string) (*ConversationMeta, bool) {
	h.mux.RLock()
	defer h.mux.RUnlock()
	m, ok := h.meta[id]
	if !ok {
		return nil, false
	}
	copy := m
	return &copy, true
}

// Children returns all direct children for parentID.
func (h *HistoryStore) Children(ctx context.Context, parentID string) ([]ConversationMeta, bool) {
	h.mux.RLock()
	defer h.mux.RUnlock()
	if parentID == "" {
		return nil, false
	}
	var out []ConversationMeta
	for _, m := range h.meta {
		if m.ParentID == parentID {
			out = append(out, m)
		}
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

// UpdateMeta applies a mutator function to the ConversationMeta identified by
// id. If the entry does not yet exist it will be created first so that the
// mutator always receives a valid pointer. The method returns true when an
// entry existed or was created, false when id is empty.
func (h *HistoryStore) UpdateMeta(ctx context.Context, id string, mutate func(*ConversationMeta)) bool {
	if id == "" || mutate == nil {
		return false
	}
	h.mux.Lock()
	defer h.mux.Unlock()
	m, ok := h.meta[id]
	if !ok {
		m = ConversationMeta{ID: id, CreatedAt: time.Now()}
	}
	mutate(&m)
	h.meta[id] = m
	return true
}
