package memory

import (
	"context"
	"strings"
	"sync"

	"github.com/tmc/langchaingo/schema"
	"github.com/tmc/langchaingo/vectorstores"
	"github.com/viant/embedius/vectordb"
	"github.com/viant/embedius/vectordb/mem"
)

// Message represents a conversation message for memory storage.
type Message struct {
	Role     string
	Content  string
	ToolName *string // Optional tool name, can be nil
}

// EmbedFunc defines a function that creates embeddings for given texts.
// It should return one embedding per input text.
type EmbedFunc func(ctx context.Context, texts []string) ([][]float32, error)

// MemoryStore defines the interface for persisting and retrieving conversation memory.
type MemoryStore interface {
	// Save stores a message under the given conversation ID.
	Save(ctx context.Context, convID string, msg Message) error
	// List retrieves all stored messages for the conversation ID in insertion order.
	List(ctx context.Context, convID string) []Message
	// Query retrieves up to max nearest messages matching the query,
	// ordered by descending similarity.
	Query(ctx context.Context, convID string, query string, max int) ([]Message, error)
}

// memoryEntry holds a stored message and its embedding.
type memoryEntry struct {
	msg       Message
	embedding []float32
}

// embedderAdapter wraps EmbedFunc to satisfy vectorstores.Find.
type embedderAdapter struct {
	f EmbedFunc
}

// EmbedQuery embeds a single query text, adapting if necessary.
func (e *embedderAdapter) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	var t string
	if strings.HasPrefix(text, "{") {
		if idx := strings.Index(text, "}"); idx != -1 {
			t = text[idx+1:]
		} else {
			t = text
		}
	} else {
		t = text
	}
	vecs, err := e.f(ctx, []string{t})
	if err != nil {
		return nil, err
	}
	if len(vecs) > 0 {
		return vecs[0], nil
	}
	return nil, nil
}

// EmbedDocuments adapts embedding calls, stripping metadata JSON if present.
func (e *embedderAdapter) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	adapted := make([]string, len(texts))
	for i, t := range texts {
		if strings.HasPrefix(t, "{") {
			if idx := strings.Index(t, "}"); idx != -1 {
				adapted[i] = t[idx+1:]
			} else {
				adapted[i] = t
			}
		} else {
			adapted[i] = t
		}
	}
	return e.f(ctx, adapted)
}

// InMemoryStore is an in-memory implementation of MemoryStore.
// It embeds messages via EmbedFunc and uses Embedius vectordb for similarity search.
type InMemoryStore struct {
	embed EmbedFunc
	data  map[string][]*memoryEntry
	mux   sync.RWMutex
	store vectordb.VectorStore
}

// NewInMemoryStore creates a new in-memory memory store using the provided EmbedFunc.
// NewInMemoryStore creates a new in-memory memory store using the provided EmbedFunc.
func NewInMemoryStore(embed EmbedFunc) *InMemoryStore {
	return &InMemoryStore{
		embed: embed,
		data:  make(map[string][]*memoryEntry),
		store: mem.NewStore(),
	}
}

// Save stores a message under the conversation ID, embedding its content.
func (s *InMemoryStore) Save(ctx context.Context, convID string, msg Message) error {
	vectors, err := s.embed(ctx, []string{msg.Content})
	if err != nil {
		return err
	}
	if len(vectors) == 0 {
		return nil
	}
	entry := &memoryEntry{
		msg:       msg,
		embedding: vectors[0],
	}
	// Add to vector store for similarity search.
	docs := []schema.Document{{
		PageContent: msg.Content,
		Metadata:    map[string]any{"role": msg.Role},
	}}
	embedder := &embedderAdapter{f: s.embed}
	if _, err := s.store.AddDocuments(ctx, docs, vectorstores.WithEmbedder(embedder), vectorstores.WithNameSpace(convID)); err != nil {
		return err
	}
	// Append to memory entries.
	s.mux.Lock()
	defer s.mux.Unlock()
	s.data[convID] = append(s.data[convID], entry)
	return nil
}

// List returns all messages stored for the conversation in insertion order.
func (s *InMemoryStore) List(ctx context.Context, convID string) []Message {
	s.mux.RLock()
	defer s.mux.RUnlock()
	entries := s.data[convID]
	result := make([]Message, len(entries))
	for i, e := range entries {
		result[i] = e.msg
	}
	return result
}

// Query retrieves up to max messages most similar to the query text.
func (s *InMemoryStore) Query(ctx context.Context, convID string, query string, max int) ([]Message, error) {
	embedder := &embedderAdapter{f: s.embed}
	docs, err := s.store.SimilaritySearch(ctx, query, max, vectorstores.WithEmbedder(embedder), vectorstores.WithNameSpace(convID))
	if err != nil {
		return nil, err
	}
	results := make([]Message, len(docs))
	for i, doc := range docs {
		var role string
		if r, ok := doc.Metadata["role"].(string); ok {
			role = r
		}
		results[i] = Message{Role: role, Content: doc.PageContent}
	}
	return results, nil
}
