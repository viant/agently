package streaming

import (
	"context"
	"sync"

	"github.com/viant/agently/genai/modelcallctx"
)

// Publisher fan-outs stream events to conversation-scoped subscribers.
type Publisher struct {
	mu   sync.RWMutex
	subs map[string]map[chan *modelcallctx.StreamEvent]struct{}
}

// NewPublisher creates a new stream publisher.
func NewPublisher() *Publisher {
	return &Publisher{subs: make(map[string]map[chan *modelcallctx.StreamEvent]struct{})}
}

// Publish implements modelcallctx.StreamPublisher.
func (p *Publisher) Publish(_ context.Context, ev *modelcallctx.StreamEvent) error {
	if p == nil || ev == nil {
		return nil
	}
	convID := ev.ConversationID
	if convID == "" {
		return nil
	}
	p.mu.RLock()
	targets := p.subs[convID]
	p.mu.RUnlock()
	for ch := range targets {
		select {
		case ch <- ev:
		default:
			// Drop if subscriber is slow to keep streaming non-blocking.
		}
	}
	return nil
}

// Subscribe returns a channel that receives events for the conversation.
func (p *Publisher) Subscribe(convID string) (<-chan *modelcallctx.StreamEvent, func()) {
	ch := make(chan *modelcallctx.StreamEvent, 128)
	if p == nil || convID == "" {
		close(ch)
		return ch, func() {}
	}
	p.mu.Lock()
	if p.subs[convID] == nil {
		p.subs[convID] = make(map[chan *modelcallctx.StreamEvent]struct{})
	}
	p.subs[convID][ch] = struct{}{}
	p.mu.Unlock()
	cancel := func() {
		p.mu.Lock()
		if subs, ok := p.subs[convID]; ok {
			delete(subs, ch)
			if len(subs) == 0 {
				delete(p.subs, convID)
			}
		}
		p.mu.Unlock()
		close(ch)
	}
	return ch, cancel
}
