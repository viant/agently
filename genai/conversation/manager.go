package conversation

import (
	"context"
	"errors"
	"github.com/google/uuid"
	agentpkg "github.com/viant/agently/genai/extension/fluxor/llm/agent"
	"github.com/viant/agently/genai/memory"
	"time"
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
	history        memory.History
	executionStore *memory.ExecutionStore
	usageStore     *memory.UsageStore
	handler        QueryHandler
	idGen          func() string
}

// UsageStore returns the in-memory usage registry attached to the manager (or nil).
func (m *Manager) UsageStore() *memory.UsageStore { return m.usageStore }

// History returns underlying memory.History implementation.
func (m *Manager) History() memory.History { return m.history }

// Messages returns the full message history for the supplied conversation ID.
// It is a thin proxy to the underlying memory.History implementation so that
// HTTP adapters (REST, WebSocket, …) can expose the history without knowing
// concrete history details.
func (m *Manager) Messages(ctx context.Context, convID string, parentId string) ([]memory.Message, error) {
	if m == nil {
		return nil, errors.New("conversation manager is nil")
	}
	if convID == "" {
		return nil, errors.New("conversation id is empty")
	}
	messages, err := m.getMessages(ctx, convID, parentId)
	if err != nil {
		return nil, err
	}
	var result = make([]memory.Message, 0, len(messages)+1)
	for _, msg := range messages {
		result = append(result, msg)
		if m.executionStore != nil {
			if exec, _ := m.executionStore.ListOutcome(ctx, convID, msg.ID); len(exec) > 0 {
				result = append(result, memory.Message{
					ID:             msg.ID + "/1",
					ConversationID: msg.ConversationID,
					ParentID:       parentId,
					Role:           "tool",
					Executions:     exec,
					CreatedAt:      msg.CreatedAt.Add(time.Second),
				})
			}
		}
	}
	return result, nil
}

func (m *Manager) getMessages(ctx context.Context, convID string, parentId string) ([]memory.Message, error) {
	result, err := m.history.GetMessages(ctx, convID)
	if err != nil {
		return nil, err
	}
	if parentId == "" {
		return result, nil
	}

	var filtered = make([]memory.Message, 0, len(result))
	for _, item := range result {
		if item.ParentID == parentId || item.ID == parentId {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
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

// WithUsageStore injects an in-memory UsageStore so that Accept() automatically
// aggregates token counts per conversation.
func WithUsageStore(s *memory.UsageStore) Option {
	return func(m *Manager) {
		m.usageStore = s
	}
}

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
func New(history memory.History, executionStore *memory.ExecutionStore, handler QueryHandler, opts ...Option) *Manager {
	if history == nil {
		history = memory.NewHistoryStore()
	}
	m := &Manager{
		history:        history,
		handler:        handler,
		executionStore: executionStore,
		usageStore:     nil,
		idGen:          uuid.NewString,
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
		// Persist error as a synthetic system message so that callers
		// polling the history can surface the failure.
		_ = m.history.AddMessage(ctx, memory.Message{
			ID:             uuid.NewString(),
			ConversationID: input.ConversationID,
			Role:           "system",
			Content:        "Error: " + err.Error(),
			CreatedAt:      time.Now(),
		})
		return nil, err
	}

	// Record token usage when a memory store is configured and handler returned
	// statistics.
	if m.usageStore != nil && output.Usage != nil {
		for _, model := range output.Usage.Keys() {
			stat := output.Usage.PerModel[model]
			if stat == nil {
				continue
			}
			m.usageStore.Add(input.ConversationID, model, stat.PromptTokens, stat.CompletionTokens, stat.EmbeddingTokens)
		}
	}
	return &output, nil
}
