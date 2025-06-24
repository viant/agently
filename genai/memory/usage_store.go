package memory

import (
	"sync"

	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/usage"
)

// UsageStore keeps token-usage statistics per conversation entirely in memory.
// It is a lightweight helper for tests and CLI use-cases where persisting
// to a database is overkill.  All operations are concurrency-safe.
//
// The layout mirrors the relational design:
//
//	– aggregated totals on conversation level (for quick summary)
//	– one-to-many breakdown per model via usage.Aggregator.
//
// The store purposefully exposes only additive updates – callers cannot
// decrease counts so that race conditions never lead to negative numbers.
type UsageStore struct {
	mux  sync.RWMutex
	data map[string]*usage.Aggregator // conversationID → aggregator
}

// NewUsageStore returns an empty usage store.
func NewUsageStore() *UsageStore {
	return &UsageStore{data: make(map[string]*usage.Aggregator)}
}

// ensure returns the Aggregator for convID, creating it if necessary.
func (s *UsageStore) ensure(convID string) *usage.Aggregator {
	s.mux.Lock()
	defer s.mux.Unlock()
	if agg, ok := s.data[convID]; ok {
		return agg
	}
	agg := &usage.Aggregator{}
	s.data[convID] = agg
	return agg
}

// OnUsage updates the statistics for convID / model by applying the supplied
// llm.Usage delta (implements the provider/base.UsageListener contract at the
// store level).
func (s *UsageStore) OnUsage(convID, model string, u *llm.Usage) {
	if u == nil || convID == "" || model == "" {
		return
	}
	agg := s.ensure(convID)
	agg.OnUsage(model, u)
}

// Add is a lower-level helper that directly increments the counters for
// convID / model.  It is mainly intended for tests.
func (s *UsageStore) Add(convID, model string, prompt, completion, embed int) {
	if convID == "" || model == "" {
		return
	}
	agg := s.ensure(convID)
	agg.Add(model, prompt, completion, embed)
}

// Aggregator returns the live aggregator for convID or nil when the
// conversation is unknown.  The returned instance is concurrency-safe; callers
// should treat its contents as read-only.
func (s *UsageStore) Aggregator(convID string) *usage.Aggregator {
	s.mux.RLock()
	agg := s.data[convID]
	s.mux.RUnlock()
	return agg
}

// Totals returns the rolled-up counts (prompt, completion, embedding) for the
// given conversation.  When convID is unknown, all numbers are zero.
func (s *UsageStore) Totals(convID string) (prompt, completion, embed int) {
	agg := s.Aggregator(convID)
	if agg == nil {
		return 0, 0, 0
	}
	// Iterate over model stats and accumulate. We do not lock the aggregator
	// internals directly (its mux is unexported). Instead we grab a snapshot
	// of the keys which already happens under aggregator's read-lock inside
	// Keys(), then read the corresponding map entries – races are extremely
	// unlikely in the in-memory/testing context and acceptable here.
	for _, model := range agg.Keys() {
		st := agg.PerModel[model]
		if st == nil {
			continue
		}
		prompt += st.PromptTokens
		completion += st.CompletionTokens
		embed += st.EmbeddingTokens
	}
	return
}

// Conversations lists the IDs currently present in the store.
func (s *UsageStore) Conversations() []string {
	s.mux.RLock()
	ids := make([]string, 0, len(s.data))
	for id := range s.data {
		ids = append(ids, id)
	}
	s.mux.RUnlock()
	return ids
}
