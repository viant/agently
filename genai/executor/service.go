package executor

import (
	"context"
	"fmt"
	"os"
	// "path/filepath"
	"strings"
	"sync/atomic"
	"time"

	clientmcp "github.com/viant/agently/adapter/mcp"
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/conversation"
	"github.com/viant/agently/genai/elicitation"
	"github.com/viant/agently/genai/embedder"
	mcpmgr "github.com/viant/agently/internal/mcp/manager"
	// "github.com/viant/agently/genai/executor/agenttool"
	// "github.com/viant/agently/genai/io/extractor"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/memory"
	agent2 "github.com/viant/agently/genai/service/agent"
	"github.com/viant/agently/genai/service/augmenter"
	"github.com/viant/agently/genai/service/core"
	"github.com/viant/agently/genai/tool"
	// plan "github.com/viant/agently/genai/tool/service/orchestration/plan"
	extfinder "github.com/viant/agently/internal/finder/extension"
	"github.com/viant/agently/internal/finder/oauth"
	hotswap "github.com/viant/agently/internal/workspace/hotswap"
	// "github.com/viant/agently/internal/loader/oauth"
	// "github.com/viant/agently/internal/workspace"
	svc "github.com/viant/agently/genai/tool/service"
	approval "github.com/viant/agently/internal/approval"
	meta "github.com/viant/agently/internal/workspace/service/meta"
	"github.com/viant/mcp-protocol/schema"
	// "github.com/viant/scy"
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

	services map[string]svc.Service

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

	// FeedSpec extension metadata finder (workspace-driven)
	extFinder *extfinder.Finder

	// oauth credentials finder
	oauthFinder *oauthfinder.Finder

	// Hot-swap manager and toggle
	hotSwap         *hotswap.Manager
	hotSwapDisabled bool

	// optional per-conversation MCP manager (injected via option)
	mcpMgr *mcpmgr.Manager

	// shared elicitation router (assistant wait path)
	elicitationRouter interface {
		RegisterByElicitationID(string, string, chan *schema.ElicitResult)
		RemoveByElicitation(string, string)
		AcceptByElicitation(string, string, *schema.ElicitResult) bool
	}

	// internal approval service (optional)
	approvalSvc approval.Service
}

// registerAgentTools exposes every agent with toolExport.expose==true as a
// Legacy hook retained (no-op) in decoupled mode.
func (e *Service) registerAgentTools() error { return nil }

// Orchestration removed in decoupled mode

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

	reg := tool.WithConversation(e.tools, memory.ConversationIDFromContext(ctx))
	res, err := reg.Execute(ctx, name, args)
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
	// no orchestration to start in decoupled mode
}

func (e *Service) IsStarted() bool {
	return atomic.LoadInt32(&e.started) == 1
}

func (e *Service) Shutdown(ctx context.Context) {
	if !atomic.CompareAndSwapInt32(&e.started, 1, 2) {
		return
	}
	// no orchestration to shutdown in decoupled mode
}

// Runtime removed in decoupled mode

// RegistryForConversation returns a Registry that is scoped to the provided
// conversation ID. Execute calls performed through the returned registry carry
// the conversation identifier in context so adapters can resolve per-conv
// resources (e.g., MCP clients, auth tokens).
func (e *Service) RegistryForConversation(convID string) tool.Registry {
	return tool.WithConversation(e.tools, convID)
}

// registerServices removed (orchestration decoupled).
func (e *Service) registerServices(_ interface{}) {
	/* Register orchestration actions: plan, execute and finalize
		// Provide MCP manager to augmenter so it can index mcp: resources.
	var upstreamConcurrency int
	var matchConcurrency int
	if e.config != nil {
		upstreamConcurrency = e.config.Default.Resources.UpstreamSyncConcurrency
		matchConcurrency = e.config.Default.Resources.MatchConcurrency
	}
	enricher := augmenter.New(
		e.embedderFinder,
		augmenter.WithMCPManager(e.mcpMgr),
		augmenter.WithUpstreamSyncConcurrency(upstreamConcurrency),
		augmenter.WithMatchConcurrency(matchConcurrency),
	)

		e.llmCore = core.New(e.modelFinder, e.tools, e.convClient)

		// Inject recorder (and keep tracer if needed later) into core so streaming execution records tool calls.
		if e.llmCore != nil {
			// Supply conversation client when ready (set later by agent service init)
		}
		// Keep core action; gate augmenter/extractor registration via env to keep them internal by default
	    _ = e.llmCore

	    // augmenter/extractor registration skipped in decoupled mode

	    // runtime not used in decoupled mode
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
	    _ = agentSvc
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

	*/
}

// ensureStore removed — executor uses clients via services directly

func (e *Service) NewContext(ctx context.Context) context.Context { return ctx }

// EventService removed in decoupled mode

// ApprovalService exposes the internal approval service when present.
func (e *Service) ApprovalService() approval.Service { return e.approvalSvc }

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

// FindToolMetadata returns ordered tool metadata (workspace extensions) that
// match the provided service/method combination. When none match it returns nil.
func (e *Service) FindToolMetadata(service, method string) []*tool.FeedSpec {
	if e == nil || e.extFinder == nil {
		return nil
	}
	return e.extFinder.FindMatches(service, method)
}

// New creates a new executor service
func New(ctx context.Context, options ...Option) (*Service, error) {
	ret := &Service{config: &Config{}, services: make(map[string]svc.Service)}
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
