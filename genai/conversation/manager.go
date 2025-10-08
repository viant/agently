package conversation

import (
	"context"
	"errors"

	"github.com/google/uuid"
	agentdef "github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/memory"
	agentpkg "github.com/viant/agently/genai/service/agent"
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
	handler  QueryHandler
	idGen    func() string
	resolver AgentResolver
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
func New(handler QueryHandler, opts ...Option) *Manager {
	// Do not create in-memory history by default; rely on recorder/domain store.
	m := &Manager{
		handler: handler,
		idGen:   uuid.NewString,
	}
	for _, o := range opts {
		o(m)
	}
	return m
}

// AgentResolver resolves agent definitions by ID during runtime.
type AgentResolver func(ctx context.Context, id string) (*agentdef.Agent, error)

// WithAgentResolver installs a resolver used by ResolveAgent.
func WithAgentResolver(r AgentResolver) Option {
	return func(m *Manager) { m.resolver = r }
}

// ResolveAgent returns the Agent configuration for the provided name using the resolver when set.
func (m *Manager) ResolveAgent(ctx context.Context, id string) (*agentdef.Agent, error) {
	if m == nil || m.resolver == nil {
		return nil, errors.New("agent resolver is not configured")
	}
	return m.resolver(ctx, id)
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

	// store the ID under both legacy (conversation) and new (convctx)
	// context keys so that all downstream code can retrieve it without
	// import cycles.
	ctx = memory.WithConversationID(ctx, input.ConversationID)
	var output agentpkg.QueryOutput
	if err := m.handler(ctx, input, &output); err != nil {
		return nil, err
	}

	return &output, nil
}
