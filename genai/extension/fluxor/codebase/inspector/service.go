package inspector

import (
	"context"
	"github.com/viant/fluxor/extension"
	"github.com/viant/fluxor/model/types"
	"github.com/viant/linager/inspector"
	"github.com/viant/linager/inspector/graph"
	"github.com/viant/linager/inspector/repository"
	"github.com/viant/x"
	"reflect"
	"strings"
	"sync"
)

const name = "codebase/inspector"

// Service extracts structured information from LLM responses
type Service struct {
	repoCache   map[string]*repository.Repository
	detectCache map[string]*repository.Project
	infoCache   map[string]*graph.Project
	mux         sync.RWMutex
}

// Input represents input for extraction
type Input struct {
	Projects []string
}

// DetectOutput represents project detection output
type DetectOutput struct {
	Projects []*repository.Project
}

// InspectOutput represents inspect output
type InspectOutput struct {
	Projects []*graph.Project
}

// Name returns the service name
func (s *Service) Name() string {
	return name
}

// Methods returns the service methods
func (s *Service) Methods() types.Signatures {
	return []types.Signature{
		{
			Name:   "detect",
			Input:  reflect.TypeOf(&Input{}),
			Output: reflect.TypeOf(&DetectOutput{}),
		},
	}
}

// Method returns the specified method
func (s *Service) Method(name string) (types.Executable, error) {
	switch strings.ToLower(name) {
	case "detect":
		return s.detect, nil
	default:
		return nil, types.NewMethodNotFoundError(name)
	}
}

func (s *Service) InitTypes(types *extension.Types) {
	types.Register(x.NewType(reflect.TypeOf(repository.Project{})))
	types.Register(x.NewType(reflect.TypeOf(graph.Project{})))
}

// detect detects projects from the provided URLs
func (s *Service) detect(ctx context.Context, in, out interface{}) error {
	input, ok := in.(*Input)
	if !ok {
		return types.NewInvalidInputError(in)
	}
	output, ok := out.(*DetectOutput)
	if !ok {
		return types.NewInvalidOutputError(out)
	}

	for _, URL := range input.Projects {

		project, err := s.Project(URL)
		if err != nil {
			return err
		}
		output.Projects = append(output.Projects, project)
	}
	return nil
}

func (s *Service) Repository(URL string) (*repository.Repository, error) {
	s.mux.RLock()
	repoInfo, ok := s.repoCache[URL]
	s.mux.RUnlock()
	if ok {
		return repoInfo, nil
	}
	repo := repository.New()
	repoInfo, err := repo.DetectRepository(URL)
	if err != nil {
		return nil, err
	}
	s.mux.Lock()
	s.repoCache[URL] = repoInfo
	s.mux.Unlock()
	return repoInfo, nil
}

func (s *Service) Project(URL string) (*repository.Project, error) {
	s.mux.RLock()
	project, ok := s.detectCache[URL]
	s.mux.RUnlock()
	if ok {
		return project, nil
	}
	repo := repository.New()
	project, err := repo.DetectProject(URL)
	if err != nil {
		return nil, err
	}
	s.mux.Lock()
	s.detectCache[URL] = project
	s.mux.Unlock()
	return project, nil
}

// inspect inspects projects from the provided URLs
func (s *Service) inspect(ctx context.Context, in, out interface{}) error {
	input, ok := in.(*Input)
	if !ok {
		return types.NewInvalidInputError(in)
	}
	output, ok := out.(*InspectOutput)
	if !ok {
		return types.NewInvalidOutputError(out)
	}

	for _, URL := range input.Projects {
		repo := repository.New()
		project, err := repo.DetectProject(URL)
		if err != nil {
			return err
		}
		factory := inspector.NewFactory(graph.DefaultConfig())
		projectInfo, err := factory.InspectProject(project)
		if err != nil {
			return err
		}
		s.infoCache[URL] = projectInfo
		output.Projects = append(output.Projects, projectInfo)
	}
	return nil
}

// New creates a new extractor service
func New() *Service {
	return &Service{
		detectCache: make(map[string]*repository.Project),
		infoCache:   make(map[string]*graph.Project),
		repoCache:   make(map[string]*repository.Repository),
	}
}
