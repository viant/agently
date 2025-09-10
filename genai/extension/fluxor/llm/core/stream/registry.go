package stream

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/mcp-protocol/syncmap"
)

var (
	ErrClosedSession  = errors.New("stream session is closed")
	ErrUnknownSession = errors.New("unknown stream session")
)

type session struct {
	handler Handler
	closed  atomic.Bool
}

type Registry struct {
	registry *syncmap.Map[string, *session]
}

func NewProvider() *Registry {
	return &Registry{
		registry: syncmap.NewMap[string, *session](),
	}
}

var provider = NewProvider()

// Register stores a session handler and returns a sessionID.
func (p *Registry) Register(h Handler) string {
	sid := uuid.NewString()
	p.registry.Put(sid, &session{handler: h})
	return sid
}

// New returns a safe event handler for the given sessionID.
// After Finish(sid) is called, the returned handler will reject further events with ErrClosedSession.
func (p *Registry) New(ctx context.Context, sessionID string) (Handler, error) {
	sess, ok := p.registry.Get(sessionID)
	if !ok {
		return nil, fmt.Errorf("failed to get handler for sessionID: %v", sessionID)
	}

	// Wrap with a guard that checks session state.
	return func(ctx context.Context, e *llm.StreamEvent) error {
		if sess.closed.Load() {
			return ErrClosedSession
		}
		return sess.handler(ctx, e)
	}, nil
}

// Finish marks the session as closed and unregisters it.
// Idempotent: calling multiple times is safe.
func (p *Registry) Finish(sessionID string) error {
	sess, ok := p.registry.Get(sessionID)
	if !ok {
		return ErrUnknownSession
	}
	if sess.closed.Swap(true) {
		// already closed
		p.registry.Delete(sessionID)
		return nil
	}
	p.registry.Delete(sessionID)
	return nil
}

func (p *Registry) Unregister(sessionID string) { _ = p.Finish(sessionID) }

func Register(h Handler) string                                  { return provider.Register(h) }
func New(ctx context.Context, sessionID string) (Handler, error) { return provider.New(ctx, sessionID) }
func Finish(sessionID string) error                              { return provider.Finish(sessionID) }
func Unregister(sessionID string)                                { provider.Unregister(sessionID) }

func PrepareStreamHandler(ctx context.Context, id string) (Handler, func(), error) {
	h, err := New(ctx, id)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create Stream handler: %w", err)
	}
	cleanup := func() { _ = Finish(id) }
	return h, cleanup, nil
}
