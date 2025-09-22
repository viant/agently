package elicitation

import (
	"context"
	"github.com/viant/agently/genai/agent/plan"
	"sync"
)

type Awaiter interface {
	AwaitElicitation(ctx context.Context, p *plan.Elicitation) (*plan.ElicitResult, error)
}

type Awaiters struct {
	newAwaiter func() Awaiter
	awaiters   map[string]Awaiter
	mux        sync.RWMutex
}

func NewAwaiters(newAwaiter func() Awaiter) *Awaiters {
	return &Awaiters{newAwaiter: newAwaiter, awaiters: map[string]Awaiter{}, mux: sync.RWMutex{}}
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
func (a *Awaiters) Lookup(key string) (Awaiter, bool) {
	a.mux.RLock()
	defer a.mux.RUnlock()
	aw, ok := a.awaiters[key]
	return aw, ok
}
func (a *Awaiters) Remove(key string) { a.mux.Lock(); defer a.mux.Unlock(); delete(a.awaiters, key) }
