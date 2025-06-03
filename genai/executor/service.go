package executor

import (
	"context"
	clientmcp "github.com/viant/agently/adapter/mcp"
	"github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/conversation"
	"github.com/viant/agently/genai/embedder"
	"github.com/viant/agently/genai/extension/fluxor/codebase/inspector"
	"github.com/viant/agently/genai/extension/fluxor/codebase/tester"
	llmagent "github.com/viant/agently/genai/extension/fluxor/llm/agent"
	"github.com/viant/agently/genai/extension/fluxor/llm/augmenter"
	"github.com/viant/agently/genai/extension/fluxor/llm/core"
	"github.com/viant/agently/genai/extension/fluxor/llm/exec"
	"github.com/viant/agently/genai/extension/fluxor/output/extractor"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/tool"
	"github.com/viant/fluxor"
	"github.com/viant/fluxor/extension"
	"github.com/viant/fluxor/model/types"
	"github.com/viant/fluxor/service/approval"
	"github.com/viant/fluxor/service/event"
	"github.com/viant/fluxor/service/meta"
	"github.com/viant/x"
	"io"
	"sync/atomic"
)

type Service struct {
	workflow       workflow
	config         *Config
	mcpClient      *clientmcp.Client
	modelFinder    llm.Finder
	modelMatcher   llm.Matcher
	embedderFinder embedder.Finder
	agentFinder    agent.Finder
	tools          *tool.Registry
	history        memory.History
	llmCore        *core.Service
	augmenter      *augmenter.Service
	agentService   *llmagent.Service
	convManager    *conversation.Manager
	metaService    *meta.Service
	started        int32

	llmLogger       io.Writer `json:"-"`
	fluxorLogWriter io.Writer `json:"-"`
}

type workflow struct {
	Options        []fluxor.Option
	Extensions     []types.Service
	Runtime        *fluxor.Runtime
	Service        *fluxor.Service
	ExtensionTypes []*x.Type
}

// AgentService returns the registered llmagent service
func (e *Service) AgentService() *llmagent.Service {
	return e.agentService
}

func (e *Service) Start(ctx context.Context) {
	if !atomic.CompareAndSwapInt32(&e.started, 0, 1) {
		return
	}
	_ = e.workflow.Service.Runtime().Start(ctx)
}

func (e *Service) IsStarted() bool {
	return atomic.LoadInt32(&e.started) == 1
}

func (e *Service) Shutdown(ctx context.Context) {
	if !atomic.CompareAndSwapInt32(&e.started, 1, 2) {
		return
	}
	_ = e.workflow.Service.Runtime().Shutdown(ctx)
}

func (e *Service) Runtime() *fluxor.Runtime {
	return e.workflow.Runtime
}

func (e *Service) registerServices() *extension.Actions {
	// Register orchestration actions: plan, execute and finalize
	defaultModel := e.config.Default.Model
	enricher := augmenter.New(e.embedderFinder)
	actions := e.workflow.Service.Actions()
	e.llmCore = core.New(e.modelFinder, e.tools, defaultModel)

	if e.llmLogger != nil {
		e.llmCore.SetLogger(e.llmLogger)
	}

	actions.Register(exec.New(e.llmCore, e.tools, defaultModel, e.ApprovalService()))
	actions.Register(enricher)
	actions.Register(e.llmCore)
	// capture actions for streaming and callbacks
	actions.Register(tester.New(actions))
	actions.Register(extractor.New())
	actions.Register(inspector.New())

	agentSvc := llmagent.New(e.llmCore, e.agentFinder, enricher, e.tools, e.workflow.Runtime, e.history)
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
	e.convManager = conversation.New(e.history, convHandler)

	return actions
}

func (e *Service) NewContext(ctx context.Context) context.Context {
	return e.workflow.Service.NewContext(ctx)
}

func (e *Service) EventService() *event.Service {
	return e.workflow.Service.EventService()
}

// ApprovalService exposes the Fluxor approval service used by the wrapped
// workflow engine. It returns nil if the service has not been initialised yet
// (which should never happen after init() succeeds).
func (e *Service) ApprovalService() approval.Service {
	if e == nil || e.workflow.Service == nil {
		return nil
	}
	return e.workflow.Service.ApprovalService()
}

// Conversation returns the shared conversation manager initialised by the service.
func (e *Service) Conversation() *conversation.Manager {
	return e.convManager
}

// LLMCore returns the underlying llm/core service instance (mainly for
// introspection or test hooks).
func (e *Service) LLMCore() *core.Service {
	return e.llmCore
}

// New creates a new executor service
func New(ctx context.Context, options ...Option) (*Service, error) {
	ret := &Service{config: &Config{}}
	for _, opt := range options {
		opt(ret)
	}
	err := ret.init(ctx)
	return ret, err
}
