package executor

import (
	"context"
	"fmt"
	clientmcp "github.com/viant/agently/adapter/mcp"
	"github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/conversation"
	"github.com/viant/agently/genai/embedder"
	llmagent "github.com/viant/agently/genai/extension/fluxor/llm/agent"
	"github.com/viant/agently/genai/extension/fluxor/llm/augmenter"
	"github.com/viant/agently/genai/extension/fluxor/llm/core"
	"github.com/viant/agently/genai/extension/fluxor/llm/exec"
	"github.com/viant/agently/genai/extension/fluxor/output/extractor"
	"github.com/viant/agently/genai/io/elicitation"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/tool"
	"github.com/viant/agently/internal/hotswap"
	"github.com/viant/fluxor"
	"github.com/viant/fluxor/extension"
	"github.com/viant/fluxor/service/approval"
	"github.com/viant/fluxor/service/event"
	"github.com/viant/fluxor/service/meta"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/viant/fluxor-mcp/mcp"
)

type Service struct {
	config         *Config
	clientHandler  *clientmcp.Client
	modelFinder    llm.Finder
	modelMatcher   llm.Matcher
	embedderFinder embedder.Finder
	agentFinder    agent.Finder
	tools          tool.Registry

	history        memory.History
	executionStore *memory.ExecutionStore
	llmCore        *core.Service

	// mcpElicitationAwaiter receives interactive user prompts when the runtime
	// encounters a schema-based elicitation request. When non-nil it is injected
	// into the internally managed MCP client so that the network round-trip can
	// be bypassed during CLI sessions or unit-tests.
	mcpElicitationAwaiter elicitation.Awaiter `json:"-"`

	augmenter    *augmenter.Service
	agentService *llmagent.Service
	convManager  *conversation.Manager
	metaService  *meta.Service
	started      int32

	// Hot-swap manager and toggle
	hotSwap         *hotswap.Manager
	hotSwapDisabled bool

	// hotSwap manages live reload of workspace resources (agents, models, etc.)

	llmLogger       io.Writer `json:"-"`
	fluxorLogWriter io.Writer `json:"-"`

	fluxorOptions []fluxor.Option
	orchestration *mcp.Service // shared fluxor-mcp service instance
}

func (e *Service) Orchestration() *mcp.Service {
	return e.orchestration
}

// ExecuteTool invokes a registered tool through the configured tool registry.
// It provides a lightweight way to run an individual tool without crafting a
// full workflow.  When `timeout` is positive a child context with that
// deadline is used.
func (e *Service) ExecuteTool(ctx context.Context, name string, args map[string]interface{}, timeout time.Duration) (interface{}, error) {
	if e == nil {
		return "", fmt.Errorf("executor: nil receiver")
	}
	if e.tools == nil {
		return "", fmt.Errorf("executor: tool registry not initialised")
	}

	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	res, err := e.tools.Execute(ctx, name, args)
	return res, err
}

// AgentService returns the registered llmagent service
func (e *Service) AgentService() *llmagent.Service {
	return e.agentService
}

func (e *Service) Start(ctx context.Context) {
	if !atomic.CompareAndSwapInt32(&e.started, 0, 1) {
		return
	}
	if e.orchestration != nil {
		_ = e.orchestration.Start(ctx)
	}
}

func (e *Service) IsStarted() bool {
	return atomic.LoadInt32(&e.started) == 1
}

func (e *Service) Shutdown(ctx context.Context) {
	if !atomic.CompareAndSwapInt32(&e.started, 1, 2) {
		return
	}
	if e.orchestration != nil {
		_ = e.orchestration.Shutdown(ctx)
	}
}

func (e *Service) Runtime() *fluxor.Runtime {
	if e.orchestration != nil {
		return e.orchestration.WorkflowRuntime()
	}
	return nil
}

func (e *Service) registerServices(actions *extension.Actions) {
	// Register orchestration actions: plan, execute and finalize
	defaultModel := e.config.Default.Model
	enricher := augmenter.New(e.embedderFinder)
	e.llmCore = core.New(e.modelFinder, e.tools, defaultModel)

	if e.llmLogger != nil {
		e.llmCore.SetLogger(e.llmLogger)
	}

	actions.Register(exec.New(e.llmCore, e.tools, defaultModel, e.ApprovalService(), e.executionStore))
	actions.Register(enricher)
	actions.Register(e.llmCore)
	// capture actions for streaming and callbacks
	actions.Register(extractor.New())

	var runtime *fluxor.Runtime
	if e.orchestration != nil {
		runtime = e.orchestration.WorkflowRuntime()
	}
	agentSvc := llmagent.New(e.llmCore, e.agentFinder, enricher, e.tools, runtime, e.history, e.executionStore, &e.config.Default)
	actions.Register(agentSvc)
	e.agentService = agentSvc

	// Initialise shared conversation manager for multi-turn interactions
	convHandler := func(ctx context.Context, in *llmagent.QueryInput, out *llmagent.QueryOutput) error {
		exec, err := agentSvc.Method("query")
		if err != nil {
			return err
		}
		return exec(ctx, in, out)
	}
	// Attach a shared in-memory usage store so that token statistics are
	// collected and later exposed via HTTP GET /v1/api/conversations/{id}/usage.
	usageStore := memory.NewUsageStore()
	e.convManager = conversation.New(e.history, e.executionStore, convHandler, conversation.WithUsageStore(usageStore))
	// Actions is modified in-place; no return value needed.
}

func (e *Service) NewContext(ctx context.Context) context.Context {
	if e.orchestration != nil {
		return e.orchestration.WorkflowService().NewContext(ctx)
	}
	return ctx
}

func (e *Service) EventService() *event.Service {
	if e.orchestration != nil {
		return e.orchestration.WorkflowService().EventService()
	}
	return nil
}

// ApprovalService exposes the Fluxor approval service from the orchestration
// engine. It returns nil when orchestration is not yet initialised.
func (e *Service) ApprovalService() approval.Service {
	if e == nil || e.orchestration == nil {
		return nil
	}
	return e.orchestration.WorkflowService().ApprovalService()
}

// Conversation returns the shared conversation manager initialised by the service.
func (e *Service) Conversation() *conversation.Manager {
	return e.convManager
}

// ExecutionStore returns the shared executoin store
func (e *Service) ExecutionStore() *memory.ExecutionStore {
	return e.executionStore
}

// LLMCore returns the underlying llm/core service instance (mainly for
// introspection or test hooks).
func (e *Service) LLMCore() *core.Service {
	return e.llmCore
}

// Config returns the underlying configuration struct (read-only). Callers
// must treat the returned pointer as immutable once the executor has been
// initialised.
func (e *Service) Config() *Config {
	return e.config
}

// New creates a new executor service
func New(ctx context.Context, options ...Option) (*Service, error) {
	ret := &Service{config: &Config{}}
	for _, opt := range options {
		opt(ret)
	}

	// Environment variable toggle (overrides default, respects explicit option)
	if !ret.hotSwapDisabled {
		if v := os.Getenv("AGENTLY_HOTSWAP"); v != "" {
			if isFalse(v) {
				ret.hotSwapDisabled = true
			}
		}
	}

	err := ret.init(ctx)
	if err != nil {
		return nil, err
	}

	// Start HotSwap when enabled --------------------------------------
	if !ret.hotSwapDisabled {
		ret.initialiseHotSwap()
	}
	return ret, nil
}

// isFalse loosely interprets various string representations of Boolean false.
func isFalse(v string) bool {
	v = strings.TrimSpace(strings.ToLower(v))
	switch v {
	case "0", "false", "no", "off":
		return true
	}
	return false
}
