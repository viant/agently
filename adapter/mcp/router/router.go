package router

import (
	"sync"

	"github.com/viant/mcp-protocol/schema"
)

// Router maps elicitation JSON‑RPC ids to waiters scoped by conversation id.
type Router struct {
	mu     sync.RWMutex
	byConv map[string]map[uint64]chan *schema.ElicitResult
}

func New() *Router { return &Router{byConv: map[string]map[uint64]chan *schema.ElicitResult{}} }

// Register installs a waiter channel for (convID, elicID).
func (r *Router) Register(elicID uint64, convID string, ch chan *schema.ElicitResult) {
	r.mu.Lock()
	if r.byConv[convID] == nil {
		r.byConv[convID] = map[uint64]chan *schema.ElicitResult{}
	}
	r.byConv[convID][elicID] = ch
	r.mu.Unlock()
}

// Resolve returns a waiter for (convID, elicID).
func (r *Router) Resolve(elicID uint64, convID string) (chan *schema.ElicitResult, bool) {
	r.mu.RLock()
	m := r.byConv[convID]
	var ch chan *schema.ElicitResult
	if m != nil {
		ch = m[elicID]
	}
	r.mu.RUnlock()
	return ch, ch != nil
}

// AcceptForConversation delivers a result to (convID, elicID) and removes it.
func (r *Router) AcceptForConversation(elicID uint64, convID string, res *schema.ElicitResult) bool {
	r.mu.Lock()
	var ch chan *schema.ElicitResult
	if m := r.byConv[convID]; m != nil {
		ch = m[elicID]
		delete(m, elicID)
		if len(m) == 0 {
			delete(r.byConv, convID)
		}
	}
	r.mu.Unlock()
	if ch == nil {
		return false
	}
	select {
	case ch <- res:
		return true
	default:
		return true
	}
}

// Remove deletes waiter for (convID, elicID).
func (r *Router) Remove(elicID uint64, convID string) {
	r.mu.Lock()
	if m := r.byConv[convID]; m != nil {
		delete(m, elicID)
		if len(m) == 0 {
			delete(r.byConv, convID)
		}
	}
	r.mu.Unlock()
}
