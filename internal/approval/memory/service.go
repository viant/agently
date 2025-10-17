package memory

import (
	"context"
	"errors"
	"sync"
	"time"

	appr "github.com/viant/agently/internal/approval"
)

type service struct {
	mu      sync.Mutex
	pending map[string]*appr.Request
	events  *eventQueue
}

func New() *service {
	return &service{pending: map[string]*appr.Request{}, events: newEventQueue()}
}

func (s *service) RequestApproval(ctx context.Context, r *appr.Request) error {
	s.mu.Lock()
	s.pending[r.ID] = r
	s.mu.Unlock()
	_ = s.events.Publish(ctx, &appr.Event{Topic: appr.TopicRequestCreated, Data: r})
	_ = s.events.Publish(ctx, &appr.Event{Topic: appr.LegacyTopicRequestNew, Data: r})
	return nil
}

func (s *service) ListPending(ctx context.Context) ([]*appr.Request, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*appr.Request, 0, len(s.pending))
	for _, r := range s.pending {
		out = append(out, r)
	}
	return out, nil
}

func (s *service) Decide(ctx context.Context, id string, approved bool, reason string) (*appr.Decision, error) {
	s.mu.Lock()
	_, ok := s.pending[id]
	if ok {
		delete(s.pending, id)
	}
	s.mu.Unlock()
	if !ok {
		return nil, errors.New("request not found")
	}
	d := &appr.Decision{ID: id, Approved: approved, Reason: reason, DecidedAt: time.Now()}
	_ = s.events.Publish(ctx, &appr.Event{Topic: appr.TopicDecisionCreated, Data: d})
	_ = s.events.Publish(ctx, &appr.Event{Topic: appr.LegacyTopicDecisionNew, Data: d})
	return d, nil
}

func (s *service) Queue() appr.Queue[appr.Event] { return s.events }

// ---------------- queue ----------------

type eventQueue struct {
	ch chan *appr.Event
}

func newEventQueue() *eventQueue { return &eventQueue{ch: make(chan *appr.Event, 128)} }

func (q *eventQueue) Publish(ctx context.Context, e *appr.Event) error {
	select {
	case q.ch <- e:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (q *eventQueue) Consume(ctx context.Context) (appr.Message[appr.Event], error) {
	select {
	case e := <-q.ch:
		return &eventMsg{e: e}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type eventMsg struct{ e *appr.Event }

func (m *eventMsg) T() *appr.Event       { return m.e }
func (m *eventMsg) Ack() error           { return nil }
func (m *eventMsg) Nack(err error) error { return nil }

var _ appr.Queue[appr.Event] = (*eventQueue)(nil)
