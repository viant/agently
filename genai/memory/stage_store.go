package memory

import (
	"sync"

	"github.com/viant/agently/genai/stage"
)

// StageStore keeps the latest Stage snapshot per conversation in memory.
// All operations are concurrency-safe.
type StageStore struct {
	mux  sync.RWMutex
	data map[string]*stage.Stage // convID → snapshot
}

// NewStageStore returns an empty store.
func NewStageStore() *StageStore {
	return &StageStore{data: make(map[string]*stage.Stage)}
}

// Set replaces the snapshot for convID.
func (s *StageStore) Set(convID string, st *stage.Stage) {
	if convID == "" || st == nil {
		return
	}
	s.mux.Lock()
	s.data[convID] = st
	s.mux.Unlock()
}

// Get returns a copy of the snapshot for convID, or nil.
func (s *StageStore) Get(convID string) *stage.Stage {
	s.mux.RLock()
	st := s.data[convID]
	s.mux.RUnlock()
	return st
}
