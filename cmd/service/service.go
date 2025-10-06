package service

import (
	"context"
	"os"
	"sync"

	afs "github.com/viant/afs"
	execpkg "github.com/viant/agently/genai/executor"
	"github.com/viant/agently/genai/llm"
	agentrepo "github.com/viant/agently/internal/repository/agent"
	extrepo "github.com/viant/agently/internal/repository/extension"
	mcprepo "github.com/viant/agently/internal/repository/mcp"
	modelrepo "github.com/viant/agently/internal/repository/model"
	oauthrepo "github.com/viant/agently/internal/repository/oauth"
	workflowrepo "github.com/viant/agently/internal/repository/workflow"
	"github.com/viant/agently/internal/workspace"
	"github.com/viant/scy"
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

	once     sync.Once
	mRepo    *modelrepo.Repository
	aRepo    *agentrepo.Repository
	wRepo    *workflowrepo.Repository
	mcpRepo  *mcprepo.Repository
	oRepo    *oauthrepo.Repository
	feedRepo *extrepo.Repository
	// tools are dynamic – no repository, expose via executor.
}

// New returns a Service using the supplied executor.Service. Ownership of
// exec is left to the caller – Service does not Stop()/Shutdown() it.
func New(exec *execpkg.Service, opts Options) *Service {
	return &Service{exec: exec, opts: opts}
}

// SwitchModel sets the default modelRef for a given agent configuration.
func (s *Service) SwitchModel(ctx context.Context, agentId, modelName string) error {
	s.initRepos()
	return s.aRepo.SwitchModel(ctx, agentId, modelName)
}

// ResetModel clears modelRef so the agent uses executor default.
func (s *Service) ResetModel(ctx context.Context, agentId string) error {
	s.initRepos()
	return s.aRepo.ResetModel(ctx, agentId)
}

// lazy init repos
func (s *Service) initRepos() {
	s.once.Do(func() {
		fs := afs.New()

		// Only populate the workspace with built-in default resources when the
		// caller did NOT override the root directory via the AGENTLY_ROOT
		// environment variable.  This behaviour keeps unit tests or callers
		// that work with a temporary/empty workspace in full control over its
		// contents.
		if os.Getenv("AGENTLY_ROOT") == "" {
			workspace.EnsureDefault(fs)
		}
		s.mRepo = modelrepo.New(fs)
		s.aRepo = agentrepo.New(fs)
		s.wRepo = workflowrepo.New(fs)
		s.mcpRepo = mcprepo.New(fs)

		s.oRepo = oauthrepo.New(fs, scy.New())
		s.feedRepo = extrepo.New(fs)
	})
}

// Repositories expose typed accessors
func (s *Service) ModelRepo() *modelrepo.Repository { s.initRepos(); return s.mRepo }
func (s *Service) AgentRepo() *agentrepo.Repository { s.initRepos(); return s.aRepo }
func (s *Service) OAuthRepo() *oauthrepo.Repository { s.initRepos(); return s.oRepo }
func (s *Service) MCPRepo() *mcprepo.Repository     { s.initRepos(); return s.mcpRepo }
func (s *Service) FeednRepo() *extrepo.Repository   { s.initRepos(); return s.feedRepo }

// ToolDefinitions returns available tool definitions (read-only) gathered from the executor’s registry.
func (s *Service) ToolDefinitions() []llm.ToolDefinition {
	if s == nil || s.exec == nil {
		return nil
	}
	core := s.exec.LLMCore()
	if core == nil {
		return nil
	}
	return core.ToolDefinitions()
}

func (s *Service) WorkflowRepo() *workflowrepo.Repository {
	s.initRepos()
	return s.wRepo
}
