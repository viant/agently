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
	elicref "github.com/viant/agently/genai/io/elicitation/refiner"
	"github.com/viant/agently/genai/service/agent/orchestrator"
	"github.com/viant/agently/genai/service/augmenter"
	"github.com/viant/agently/genai/service/core"
	elicitation "github.com/viant/agently/genai/service/elicitation"
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

	// refinerSvc refines elicitation schemas for improved UX (ordering, widgets, defaults).
	refinerSvc elicref.Service

	elicition *elicitation.Service
}

// SetRuntime sets the fluxor runtime for orchestration
func (s *Service) SetRuntime(rt *fluxor.Runtime) {
	s.runtime = rt
}

// WithElicitationRefiner injects a refiner service used to enhance elicitation
// schemas before they are presented to the user.
func WithElicitationRefiner(r elicref.Service) Option {
	return func(s *Service) { s.refinerSvc = r }
}

// New creates a new agent service instance with the given tool registry and fluxor runtime
func New(llm *core.Service, agentFinder agent.Finder, augmenter *augmenter.Service, registry tool.Registry,
	runtime *fluxor.Runtime,
	defaults *config.Defaults,
	convClient apiconv.Client,

	opts ...Option) *Service {
	srv := &Service{
		defaults:    defaults,
		llm:         llm,
		agentFinder: agentFinder,
		augmenter:   augmenter,
		registry:    registry,
		runtime:     runtime,
		convClient:  convClient,
		fs:          afs.New(),
	}

	for _, o := range opts {
		o(srv)
	}
	// Default elicitation refiner when none provided: use workspace preset refiner.
	if srv.refinerSvc == nil {
		srv.refinerSvc = elicref.DefaultService{}
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

	// Initialize shared elicitation util holder
	if srv.convClient != nil {
		// Pass the same refiner into elicitation service so all refinements are centralized
		srv.elicition = elicitation.New(srv.convClient, elicitation.WithRefiner(srv.refinerSvc))
	}

	return srv
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
