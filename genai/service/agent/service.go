// Package agent coordinates an agent turn across multiple responsibilities
package agent

import (
	"context"
	"reflect"
	"strings"

	"github.com/viant/afs"
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/agent"
	cancels "github.com/viant/agently/genai/conversation/cancel"
	elicitation "github.com/viant/agently/genai/elicitation"
	elicrouter "github.com/viant/agently/genai/elicitation/router"
	"github.com/viant/agently/genai/executor/config"
	"github.com/viant/agently/genai/service/augmenter"
	"github.com/viant/agently/genai/service/core"
	"github.com/viant/agently/genai/tool"
	svc "github.com/viant/agently/genai/tool/service"
	mcpmgr "github.com/viant/agently/internal/mcp/manager"
	orchestrator "github.com/viant/agently/internal/service/agent/orchestrator"
	implconv "github.com/viant/agently/internal/service/conversation"
)

// Option customises Service instances.
type Option func(*Service)

const (
	name = "llm/agent"
)

type Service struct {
	llm          *core.Service
	registry     tool.Registry
	fs           afs.Service
	agentFinder  agent.Finder
	augmenter    *augmenter.Service
	orchestrator *orchestrator.Service

	defaults *config.Defaults

	// conversation is a shared conversation client used to fetch transcript/usage.
	conversation apiconv.Client

	elicitation *elicitation.Service
	// Backward-compatible fields for wiring; passed into elicitation service
	elicRouter     elicrouter.ElicitationRouter
	awaiterFactory func() elicitation.Awaiter

	// Optional cancel registry used to expose per-turn cancel functions to
	// external actors (e.g., HTTP or UI) without creating multiple cancel scopes.
	cancelReg cancels.Registry

	// Optional MCP client manager to resolve resources via MCP servers.
	mcpMgr *mcpmgr.Manager
}

func (s *Service) Finder() agent.Finder {
	return s.agentFinder
}

// SetRuntime removed: orchestration decoupled

// WithElicitationRouter injects a router to coordinate elicitation waits
// for assistant-originated prompts. When set, the agent will register a
// waiter and block until the HTTP/UI handler completes the elicitation.
func WithElicitationRouter(r elicrouter.ElicitationRouter) Option {
	return func(s *Service) { s.elicRouter = r }
}

// WithNewElicitationAwaiter configures a local awaiter used to resolve
// assistant-originated elicitations in interactive environments (CLI).
func WithNewElicitationAwaiter(newAwaiter func() elicitation.Awaiter) Option {
	return func(s *Service) { s.awaiterFactory = newAwaiter }
}

// WithCancelRegistry injects a registry to register per-turn cancel functions
// when executing Agent.Query. When nil, cancel registration is skipped.
func WithCancelRegistry(reg cancels.Registry) Option {
	return func(s *Service) { s.cancelReg = reg }
}

// WithMCPManager attaches an MCP Manager to resolve resources via MCP servers.
func WithMCPManager(m *mcpmgr.Manager) Option { return func(s *Service) { s.mcpMgr = m } }

// New creates a new agent service instance with the given tool registry.
func New(llm *core.Service, agentFinder agent.Finder, augmenter *augmenter.Service, registry tool.Registry,
	defaults *config.Defaults,
	convClient apiconv.Client,

	opts ...Option) *Service {
	srv := &Service{
		defaults:     defaults,
		llm:          llm,
		agentFinder:  agentFinder,
		augmenter:    augmenter,
		registry:     registry,
		conversation: convClient,
		fs:           afs.New(),
		cancelReg:    cancels.Default(),
	}

	for _, o := range opts {
		o(srv)
	}
	// Instantiate conversation API once; ignore errors to preserve backward compatibility
	if dao, err := implconv.NewDatly(context.Background()); err == nil {
		if cli, err := implconv.New(context.Background(), dao); err == nil {
			srv.conversation = cli
		}
	}
	// Wire core and orchestrator with conversation client
	if srv.conversation != nil && srv.llm != nil {
		srv.llm.SetConversationClient(srv.conversation)
		srv.orchestrator = orchestrator.New(llm, registry, srv.conversation)
	}

	// Initialize orchestrator with conversation client
	srv.orchestrator = orchestrator.New(llm, registry, srv.conversation)
	srv.elicitation = elicitation.New(srv.conversation, nil, srv.elicRouter, srv.awaiterFactory)

	return srv
}

// Name returns the service name
func (s *Service) Name() string {
	return name
}

// Methods returns the service methods
func (s *Service) Methods() svc.Signatures {
	return []svc.Signature{
		{
			Name:   "query",
			Input:  reflect.TypeOf(&QueryInput{}),
			Output: reflect.TypeOf(&QueryOutput{}),
		},
	}
}

// Method returns the specified method
func (s *Service) Method(name string) (svc.Executable, error) {
	switch strings.ToLower(name) {
	case "query":
		return s.query, nil
	default:
		return nil, svc.NewMethodNotFoundError(name)
	}
}
