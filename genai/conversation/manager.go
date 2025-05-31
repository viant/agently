package conversation

import (
	"context"
	"errors"
	"github.com/google/uuid"
	agentpkg "github.com/viant/agently/genai/extension/fluxor/llm/agent"
	"github.com/viant/agently/genai/memory"
)

// QueryHandler is a thin adapter used by the Manager to delegate the actual
// query processing (LLM, tool-orchestration, etc.). A production implementation
// would typically be a wrapper around *agent.Service, but tests can provide a
// lightweight stub.
type QueryHandler func(ctx context.Context, input *agentpkg.QueryInput, output *agentpkg.QueryOutput) error

// Manager coordinates multi-turn conversations. It keeps track of
// conversation history via the provided memory.History and delegates the heavy
// lifting to a QueryHandler.
//
// The goal is to decouple I/O adapters (CLI, REST, WebSocket, …) from the
// underlying agent orchestration so that they only need to call Accept().
type Manager struct {
	history memory.History
	handler QueryHandler
	idGen   func() string
}

// List returns all known conversation IDs.
func (m *Manager) List(ctx context.Context) ([]string, error) {
	if m == nil {
		return nil, errors.New("conversation manager is nil")
	}
	if lister, ok := m.history.(interface {
		ListIDs(ctx context.Context) ([]string, error)
	}); ok {
		return lister.ListIDs(ctx)
	}
	return nil, errors.New("underlying history store does not support listing")
}

// Delete removes an entire conversation by ID.
func (m *Manager) Delete(ctx context.Context, id string) error {
	if m == nil {
		return errors.New("conversation manager is nil")
	}
	if deleter, ok := m.history.(interface {
		Delete(ctx context.Context, convID string) error
	}); ok {
		return deleter.Delete(ctx, id)
	}
	return errors.New("underlying history store does not support delete")
}

// Option allows to customise Manager behaviour.
type Option func(*Manager)

// WithIDGenerator overrides the default conversation-ID generator.
func WithIDGenerator(f func() string) Option {
	return func(m *Manager) {
		if f != nil {
			m.idGen = f
		}
	}
}

// New returns a new Manager instance. If history is nil an in-memory store is
// created. If idGen is not supplied uuid.NewString() is used.
func New(history memory.History, handler QueryHandler, opts ...Option) *Manager {
	if history == nil {
		history = memory.NewHistoryStore()
	}
	m := &Manager{
		history: history,
		handler: handler,
		idGen:   uuid.NewString,
	}
	for _, o := range opts {
		o(m)
	}
	return m
}

// Accept processes a user query within a conversation.
//  1. Ensures ConversationID is present (generates if empty).
//  2. Delegates to the configured QueryHandler.
//  3. Returns the handler's output.
func (m *Manager) Accept(ctx context.Context, input *agentpkg.QueryInput) (*agentpkg.QueryOutput, error) {
	if m == nil {
		return nil, errors.New("conversation manager is nil")
	}
	if m.handler == nil {
		return nil, errors.New("query handler is nil")
	}
	if input == nil {
		return nil, errors.New("input is nil")
	}

	// Guarantee we have a conversation ID so that downstream services can
	// track history.
	if input.ConversationID == "" {
		input.ConversationID = m.idGen()
	}

	var output agentpkg.QueryOutput
	if err := m.handler(ctx, input, &output); err != nil {
		return nil, err
	}
	return &output, nil
}
