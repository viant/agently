package awaitreg

// Lightweight, dependency-free per-conversation Awaiter registry.  The package
// intentionally does *not* import conversation, elicitation or any other
// higher-level module so that it can be used from all layers without cyclical
// imports.

import (
	"sync"

	"github.com/viant/agently/genai/io/elicitation"
)

// Registry maps conversation-ID → Awaiter.  Awaiters are created lazily via
// the supplied factory.  The zero value is not usable – call New.
type Registry struct {
	factory func() elicitation.Awaiter
	mux     sync.RWMutex
	items   map[string]elicitation.Awaiter
}

// New returns an initialised registry that uses factory to instantiate a new
// Awaiter the first time a conversation requests one.
func New(factory func() elicitation.Awaiter) *Registry {
	if factory == nil {
		panic("awaitreg: factory must not be nil")
	}
	return &Registry{factory: factory, items: map[string]elicitation.Awaiter{}}
}

// Ensure returns the Awaiter bound to convID, creating it on-demand when it
// does not yet exist.
func (r *Registry) Ensure(convID string) elicitation.Awaiter {
	if r == nil {
		return nil
	}
	r.mux.RLock()
	aw, ok := r.items[convID]
	r.mux.RUnlock()
	if ok {
		return aw
	}
	r.mux.Lock()
	defer r.mux.Unlock()
	if aw, ok = r.items[convID]; ok {
		return aw
	}
	aw = r.factory()
	r.items[convID] = aw
	return aw
}

// Remove deletes the Awaiter for convID – callers should invoke this when a
// conversation is closed so that resources are freed.
func (r *Registry) Remove(convID string) {
	if r == nil {
		return
	}
	r.mux.Lock()
	delete(r.items, convID)
	r.mux.Unlock()
}
