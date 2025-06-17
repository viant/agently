package service

import (
	"context"
	afs "github.com/viant/afs"
	execpkg "github.com/viant/agently/genai/executor"
	agentrepo "github.com/viant/agently/internal/repository/agent"
	mcprepo "github.com/viant/agently/internal/repository/mcp"
	modelrepo "github.com/viant/agently/internal/repository/model"
	workflowrepo "github.com/viant/agently/internal/repository/workflow"
	"github.com/viant/agently/internal/workspace"
	"sync"
)

// Options configures behaviour of Service.
type Options struct {
	Interaction InteractionHandler // optional
}

// Service exposes high-level operations (currently Chat) that are decoupled
// from any particular user-interface.
type Service struct {
	exec *execpkg.Service
	opts Options

	once    sync.Once
	mRepo   *modelrepo.Repository
	aRepo   *agentrepo.Repository
	wRepo   *workflowrepo.Repository
	mcpRepo *mcprepo.Repository
}

// New returns a Service using the supplied executor.Service. Ownership of
// exec is left to the caller – Service does not Stop()/Shutdown() it.
func New(exec *execpkg.Service, opts Options) *Service {
	return &Service{exec: exec, opts: opts}
}

// SwitchModel sets the default modelRef for a given agent configuration.
func (s *Service) SwitchModel(ctx context.Context, agentName, modelName string) error {
	s.initRepos()
	return s.aRepo.SwitchModel(ctx, agentName, modelName)
}

// ResetModel clears modelRef so the agent uses executor default.
func (s *Service) ResetModel(ctx context.Context, agentName string) error {
	s.initRepos()
	return s.aRepo.ResetModel(ctx, agentName)
}

// lazy init repos
func (s *Service) initRepos() {
	s.once.Do(func() {
		fs := afs.New()
		// auto-bootstrap workspace with default resources when empty
		workspace.EnsureDefault(fs)
		s.mRepo = modelrepo.New(fs)
		s.aRepo = agentrepo.New(fs)
		s.wRepo = workflowrepo.New(fs)
		s.mcpRepo = mcprepo.New(fs)
	})
}

// Repositories expose typed accessors
func (s *Service) ModelRepo() *modelrepo.Repository       { s.initRepos(); return s.mRepo }
func (s *Service) AgentRepo() *agentrepo.Repository       { s.initRepos(); return s.aRepo }
func (s *Service) WorkflowRepo() *workflowrepo.Repository { s.initRepos(); return s.wRepo }
func (s *Service) MCPRepo() *mcprepo.Repository           { s.initRepos(); return s.mcpRepo }
