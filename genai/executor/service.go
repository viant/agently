package executor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	clientmcp "github.com/viant/agently/adapter/mcp"
	mcpmgr "github.com/viant/agently/adapter/mcp/manager"
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/conversation"
	"github.com/viant/agently/genai/elicitation"
	"github.com/viant/agently/genai/embedder"
	"github.com/viant/agently/genai/executor/agenttool"
	"github.com/viant/agently/genai/io/extractor"
	"github.com/viant/agently/genai/llm"
	agent2 "github.com/viant/agently/genai/service/agent"
	"github.com/viant/agently/genai/service/augmenter"
	"github.com/viant/agently/genai/service/core"
	"github.com/viant/agently/genai/tool"
	"github.com/viant/agently/internal/finder/oauth"
	"github.com/viant/agently/internal/hotswap"
	"github.com/viant/agently/internal/loader/oauth"
	"github.com/viant/agently/internal/workspace"
	"github.com/viant/fluxor"
	"github.com/viant/fluxor/extension"
	"github.com/viant/fluxor/service/approval"
	"github.com/viant/fluxor/service/event"
	"github.com/viant/fluxor/service/meta"
	"github.com/viant/mcp-protocol/schema"
	"github.com/viant/scy"

	"github.com/viant/fluxor-mcp/mcp"
)

type Service struct {
	config         *Config
	clientHandler  *clientmcp.Client
	convClient     apiconv.Client
	modelFinder    llm.Finder
	modelMatcher   llm.Matcher
	embedderFinder embedder.Finder
	agentFinder    agent.Finder
	tools          tool.Registry

	llmCore *core.Service

	// newAwaiter receives interactive user prompts when the runtime
	// encounters a schema-based elicitation request. When non-nil it is injected
	// into the internally managed MCP client so that the network round-trip can
	// be bypassed during CLI sessions or unit-tests.
	newAwaiter func() elicitation.Awaiter `json:"-"`

	augmenter    *augmenter.Service
	agentService *agent2.Service
	convManager  *conversation.Manager
	metaService  *meta.Service
	started      int32

	// oauth credentials finder
	oauthFinder *oauthfinder.Finder

	// Hot-swap manager and toggle
	hotSwap         *hotswap.Manager
	hotSwapDisabled bool

	fluxorOptions []fluxor.Option
	orchestration *mcp.Service // shared fluxor-mcp service instance

	// optional per-conversation MCP manager (injected via option)
	mcpMgr *mcpmgr.Manager

	// shared elicitation router (assistant wait path)
	elicitationRouter interface {
		RegisterByElicitationID(string, string, chan *schema.ElicitResult)
		RemoveByElicitation(string, string)
		AcceptByElicitation(string, string, *schema.ElicitResult) bool
	}
}

// registerAgentTools exposes every agent with toolExport.expose==true as a
// Fluxor-MCP service/method within the shared orchestration container. It must
// be invoked after e.orchestration is initialised.
func (e *Service) registerAgentTools() error {
	if e.orchestration == nil {
		return nil
	}

	// Allow disabling agent tool exposure via env flag to reduce tool surface.
	// Default: disabled (only core/system/sqlkit tools visible via registry filter).
	if strings.TrimSpace(os.Getenv("AGENTLY_EXPOSE_AGENT_TOOLS")) == "0" || os.Getenv("AGENTLY_EXPOSE_AGENT_TOOLS") == "" {
		return nil
	}

	actions := e.orchestration.WorkflowService().Actions()

	for _, ag := range e.config.Agent.Items {
		if ag == nil || ag.ToolExport == nil || !ag.ToolExport.Expose {
			continue
		}

		svcName := ag.ToolExport.Service
		if svcName == "" {
			svcName = fmt.Sprintf("agentExec") // default shared service
		}

		method := ag.ToolExport.Method
		if method == "" {
			method = ag.ID // unique per agent in shared service
		}

		existing := actions.Lookup(svcName)
		var gs *agenttool.GroupService
		if existing == nil {
			gs = agenttool.NewGroupService(svcName, e.orchestration)
			if err := actions.Register(gs); err != nil {
				return fmt.Errorf("register service %s: %w", svcName, err)
			}
		} else {
			var ok bool
			gs, ok = existing.(*agenttool.GroupService)
			if !ok {
				return fmt.Errorf("service name clash for %s", svcName)
			}
		}

		gs.Add(method, ag)
	}
	return nil
}

func (e *Service) Orchestration() *mcp.Service {
	return e.orchestration
}

// OAuthFinder exposes decrypted OAuth2 configurations loaded from workspace.
func (e *Service) OAuthFinder() *oauthfinder.Finder { return e.oauthFinder }

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
func (e *Service) AgentService() *agent2.Service {
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
	enricher := augmenter.New(e.embedderFinder)

	e.llmCore = core.New(e.modelFinder, e.tools, e.convClient)

	// Inject recorder (and keep tracer if needed later) into core so streaming execution records tool calls.
	if e.llmCore != nil {
		// Supply conversation client when ready (set later by agent service init)
	}
	// Keep core action; gate augmenter/extractor registration via env to keep them internal by default
	actions.Register(e.llmCore)
	if strings.TrimSpace(os.Getenv("AGENTLY_REGISTER_AUGMENTER")) == "1" {
		actions.Register(enricher)
	}
	if strings.TrimSpace(os.Getenv("AGENTLY_REGISTER_EXTRACTOR")) == "1" {
		actions.Register(extractor.New())
	}

	var runtime *fluxor.Runtime
	if e.orchestration != nil {
		runtime = e.orchestration.WorkflowRuntime()
	}
	agentSvc := agent2.New(
		e.llmCore, e.agentFinder, enricher, e.tools, runtime, &e.config.Default, e.convClient,
		// Inject elicitation router for LLM waits
		func(s *agent2.Service) {
			if e.elicitationRouter != nil {
				agent2.WithElicitationRouter(e.elicitationRouter)(s)
			}
		},
		// Inject interactive awaiter (CLI) so assistant elicitations can resolve without UI
		func(s *agent2.Service) {
			if e.newAwaiter != nil {
				agent2.WithNewElicitationAwaiter(e.newAwaiter)(s)
			}
		},
	)
	actions.Register(agentSvc)
	e.agentService = agentSvc

	// Configure runagent defaults derived from executor config.
	// The run executable is now part of agent.Service; configure via its options above.

	// ------------------- OAuth configs -------------------
	if e.oauthFinder == nil {
		e.oauthFinder = oauthfinder.New()
	}
	// Use shared scy service (blowfish default key)
	scySvc := scy.New()
	loader := oauthloader.New(scySvc)

	base := e.config.BaseURL
	if strings.TrimSpace(base) == "" {
		base = workspace.Root()
	}
	oauthDir := filepath.Join(base, "oauth")
	_ = filepath.WalkDir(oauthDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return nil
		}
		cfg, err := loader.Load(context.Background(), path)
		if err != nil {
			fmt.Printf("warn: oauth load %s: %v\n", path, err)
			return nil
		}
		e.oauthFinder.AddConfig(cfg.Name, cfg)
		return nil
	})

	// Register OAuth hot-swap adaptor so edits on YAML refresh finder.
	if e.hotSwap != nil && !e.hotSwapDisabled {
		e.hotSwap.Register(workspace.KindOAuth, hotswap.NewOAuthAdaptor(loader, e.oauthFinder))
	}

	// Initialise shared conversation manager for multi-turn interactions
	convHandler := func(ctx context.Context, in *agent2.QueryInput, out *agent2.QueryOutput) error {
		exec, err := agentSvc.Method("query")
		if err != nil {
			return err
		}
		return exec(ctx, in, out)
	}

	e.convManager = conversation.New(convHandler)
	// Actions is modified in-place; no return value needed.
}

// ensureStore removed — executor uses clients via services directly

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

	// StartedAt HotSwap when enabled --------------------------------------
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
