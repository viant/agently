package tester

import (
	"context"
	"github.com/viant/agently/genai/extension/fluxor/codebase/inspector"
	"github.com/viant/fluxor/extension"
	"github.com/viant/fluxor/model/types"
	"github.com/viant/fluxor/service/action/system/exec"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
)

type Service struct {
	exec      *exec.Service
	inspector *inspector.Service
	actions   *extension.Actions
	mux       sync.RWMutex
}

const name = "codebase/tester"

type Input struct {
	Paths []string `json:"paths,omitempty"`
}

// Output represents output from extraction
type Output struct {
	Status int
	Output string
}

// New creates a new extractor service
func New(actions *extension.Actions) *Service {
	return &Service{
		actions: actions,
	}
}

// Name returns the service name
func (s *Service) Name() string {
	return name
}

// Methods returns the service methods
func (s *Service) Methods() types.Signatures {
	return []types.Signature{
		{
			Name:   "test",
			Input:  reflect.TypeOf(&Input{}),
			Output: reflect.TypeOf(&Output{}),
		},
	}
}

// Method returns the specified method
func (s *Service) Method(name string) (types.Executable, error) {
	switch strings.ToLower(name) {
	case "test":
		return s.test, nil
	default:
		return nil, types.NewMethodNotFoundError(name)
	}
}

// test processes LLM responses to print structured data
func (s *Service) test(ctx context.Context, in, out interface{}) error {
	input, ok := in.(*Input)
	if !ok {
		return types.NewInvalidInputError(in)
	}
	output, ok := out.(*Output)
	if !ok {
		return types.NewInvalidOutputError(out)
	}
	if len(input.Paths) == 0 {
		return nil
	}

	var testables = map[string]*Testable{}
	anInspector := s.ensureInspector()
	for _, aPath := range input.Paths {
		project, err := anInspector.Project(aPath)
		if err != nil {
			continue
		}
		testable, ok := testables[project.RootPath]
		if !ok {
			testable = &Testable{
				Project:  project,
				Packages: map[string]bool{},
			}
		}
		if pkg, err := filepath.Rel(project.RootPath, aPath); err == nil {
			testable.Packages[pkg] = true
		}
	}

	if len(testables) == 0 {
		return nil
	}

	for _, testable := range testables {
		if err := s.testProject(ctx, testable, output); err != nil || output.Status != 0 {
			return err
		}
	}
	return nil
}

func (s *Service) testProject(ctx context.Context, testable *Testable, output *Output) error {
	var err error
	for pkg := range testable.Packages {
		err = s.testPackage(ctx, testable, pkg, output)
		if err != nil || output.Status != 0 {
			return err
		}
	}
	return nil

}

func (s *Service) ensureExecutor() *exec.Service {
	s.mux.RLock()
	ret := s.exec
	s.mux.RUnlock()
	if ret != nil {
		return ret
	}
	s.mux.Lock()
	ret = extension.LookupService[*exec.Service](s.actions)
	s.exec = ret
	s.mux.Unlock()
	return ret
}

func (s *Service) ensureInspector() *inspector.Service {
	s.mux.RLock()
	ret := s.inspector
	s.mux.RUnlock()
	if ret != nil {
		return ret
	}
	s.mux.Lock()
	ret = extension.LookupService[*inspector.Service](s.actions)
	s.inspector = ret
	s.mux.Unlock()
	return ret
}

func (s *Service) testPackage(ctx context.Context, testable *Testable, pkg string, output *Output) error {
	if goMod := testable.Project.GoModule; goMod != nil && goMod.Mod.Path != "" {
		pkg = path.Join(goMod.Mod.Path, pkg)
	}
	enExecutor := s.ensureExecutor()
	var commands = []string{
		"cd " + testable.Project.RootPath,
		"go test -v " + pkg}
	execOutput := &exec.Output{}
	err := enExecutor.Execute(ctx, &exec.Input{
		Commands: commands,
	}, execOutput)
	if err != nil {
		return err
	}
	output.Status = execOutput.Status
	output.Output += execOutput.Stdout
	return nil
}
