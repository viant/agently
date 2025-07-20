package elicitation

// Package elicitation groups types that deal with interactive collection of a
// JSON-document that fulfils a given JSON-schema.

import (
	"context"
	"github.com/viant/agently/genai/agent/plan"
	"sync"
)

// Awaiter waits for a user (or other UI component) to either provide a
// payload that satisfies the supplied schema or decline the request.
//
// The interface is intentionally minimal so that it can be implemented by a
// variety of front-ends (CLI, HTTP, GUI, tests, …) without dragging in large
// dependencies.
type Awaiter interface {
	// AwaitElicitation blocks until the user accepts (or declines) the
	// elicitation request. It must honour ctx for cancellation.
	AwaitElicitation(ctx context.Context, p *plan.Elicitation) (*plan.ElicitResult, error)
}

type Awaiters struct {
	newAwaiter func() Awaiter
	awaiters   map[string]Awaiter //awaiter per conversation
	mux        sync.RWMutex
}

// NewAwaiters returns new awaiters
func NewAwaiters(newAwaiter func() Awaiter) *Awaiters {
	return &Awaiters{
		newAwaiter: newAwaiter,
		awaiters:   map[string]Awaiter{},
		mux:        sync.RWMutex{},
	}
}

func (a *Awaiters) Ensure(key string) Awaiter {
	a.mux.Lock()
	defer a.mux.Unlock()
	aw, ok := a.awaiters[key]
	if !ok {
		aw = a.newAwaiter()
		a.awaiters[key] = aw
	}
	return aw
}

// EnsureContext retrieves (or lazily creates) the Awaiter associated with the
// conversation referenced in ctx. It falls back to an empty-string key when
// ctx does not carry a conversation ID so behaviour stays compatible with
// previous global singleton usage.
// EnsureContext is implemented in the registry wrapper to avoid circular
// dependencies between the conversation and elicitation packages.

func (a *Awaiters) Lookup(key string) (Awaiter, bool) {
	a.mux.RLock()
	defer a.mux.RUnlock()
	aw, ok := a.awaiters[key]
	return aw, ok
}

// Remove deletes an Awaiter by key
func (a *Awaiters) Remove(key string) {
	a.mux.Lock()
	defer a.mux.Unlock()
	delete(a.awaiters, key)
}
