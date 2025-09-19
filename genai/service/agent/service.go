// Package agent coordinates an agent turn across multiple responsibilities
package agent

import (
	"context"
	"reflect"
	"strings"

	"github.com/viant/afs"
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/executor/config"
	"github.com/viant/agently/genai/service/agent/orchestrator"
	"github.com/viant/agently/genai/service/augmenter"
	"github.com/viant/agently/genai/service/core"
	"github.com/viant/agently/genai/tool"
	implconv "github.com/viant/agently/internal/service/conversation"
	"github.com/viant/fluxor"
	"github.com/viant/fluxor/model/types"
)

// Option customises Service instances.
type Option func(*Service)

const (
	name = "llm/agent"
)

type Service struct {
	llm         *core.Service
	registry    tool.Registry
	fs          afs.Service
	agentFinder agent.Finder
	augmenter   *augmenter.Service
	// Runtime is the shared fluxor workflow runtime for orchestration
	runtime      *fluxor.Runtime
	orchestrator *orchestrator.Service

	defaults *config.Defaults

	// convClient is a shared conversation client used to fetch transcript/usage.
	convClient apiconv.Client
}

// SetRuntime sets the fluxor runtime for orchestration
func (s *Service) SetRuntime(rt *fluxor.Runtime) {
	s.runtime = rt
}

// New creates a new agent service instance with the given tool registry and fluxor runtime
func New(llm *core.Service, agentFinder agent.Finder, augmenter *augmenter.Service, registry tool.Registry,
	runtime *fluxor.Runtime,
	defaults *config.Defaults, opts ...Option) *Service {
	srv := &Service{
		defaults:    defaults,
		llm:         llm,
		agentFinder: agentFinder,
		augmenter:   augmenter,
		registry:    registry,
		runtime:     runtime,
		fs:          afs.New(),
	}

	for _, o := range opts {
		o(srv)
	}
	// Instantiate conversation API once; ignore errors to preserve backward compatibility
	if dao, err := implconv.NewDatly(context.Background()); err == nil {
		if cli, err := implconv.New(context.Background(), dao); err == nil {
			srv.convClient = cli
		}
	}
	// Wire core and orchestrator with conversation client
	if srv.convClient != nil && srv.llm != nil {
		srv.llm.SetConversationClient(srv.convClient)
		srv.orchestrator = orchestrator.New(llm, registry, srv.convClient)
	}

	// Initialize orchestrator with conversation client
	srv.orchestrator = orchestrator.New(llm, registry, srv.convClient)

	return srv
}

// WithConversationAPI injects a shared conversation API instance.
func WithConversationClient(cli apiconv.Client) Option {
	return func(s *Service) { s.convClient = cli }
}

// Name returns the service name
func (s *Service) Name() string {
	return name
}

// Methods returns the service methods
func (s *Service) Methods() types.Signatures {
	return []types.Signature{
		{
			Name:   "query",
			Input:  reflect.TypeOf(&QueryInput{}),
			Output: reflect.TypeOf(&QueryOutput{}),
		},
	}
}

// Method returns the specified method
func (s *Service) Method(name string) (types.Executable, error) {
	switch strings.ToLower(name) {
	case "query":
		return s.query, nil
	default:
		return nil, types.NewMethodNotFoundError(name)
	}
}
