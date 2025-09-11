// Package agent coordinates an agent turn across multiple responsibilities
package agent

import (
	"reflect"
	"strings"

	"github.com/viant/afs"
	"github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/executor/config"
	"github.com/viant/agently/genai/service/agent/orchestrator"
	"github.com/viant/agently/genai/service/augmenter"
	"github.com/viant/agently/genai/service/core"
	"github.com/viant/agently/genai/tool"
	"github.com/viant/agently/internal/domain"
	"github.com/viant/agently/internal/domain/recorder"
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

	recorder recorder.Recorder
	store    domain.Store
	defaults *config.Defaults
}

// SetRuntime sets the fluxor runtime for orchestration
func (s *Service) SetRuntime(rt *fluxor.Runtime) {
	s.runtime = rt
}

// New creates a new agent service instance with the given tool registry and fluxor runtime
func New(llm *core.Service, agentFinder agent.Finder, augmenter *augmenter.Service, registry tool.Registry,
	runtime *fluxor.Runtime,
	recorder recorder.Recorder,
	store domain.Store,
	defaults *config.Defaults, opts ...Option) *Service {
	srv := &Service{
		defaults:     defaults,
		llm:          llm,
		agentFinder:  agentFinder,
		augmenter:    augmenter,
		recorder:     recorder,
		registry:     registry,
		runtime:      runtime,
		store:        store,
		orchestrator: orchestrator.New(llm, registry, recorder),
		fs:           afs.New(),
	}
	for _, o := range opts {
		o(srv)
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
