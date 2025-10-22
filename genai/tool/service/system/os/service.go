package sysos

import (
	"context"
	"os"
	"reflect"
	"strings"

	svc "github.com/viant/agently/genai/tool/service"
)

// Name identifies this service for MCP routing.
const Name = "system/os"

// Service exposes OS-related helper functions.
type Service struct{}

// New creates a new Service instance.
func New() *Service { return &Service{} }

// GetEnvInput specifies environment variable names to read.
type GetEnvInput struct {
	Names []string `json:"names" description:"Names of environment variables to read"`
}

// GetEnvOutput returns values for variables that exist.
type GetEnvOutput struct {
	Values map[string]string `json:"values"`
}

// Name returns the service name.
func (s *Service) Name() string { return Name }

// Methods returns supported method signatures.
func (s *Service) Methods() svc.Signatures {
	return []svc.Signature{{
		Name:        "getEnv",
		Description: "Gets environment variables for provided names",
		Input:       reflect.TypeOf(&GetEnvInput{}),
		Output:      reflect.TypeOf(&GetEnvOutput{}),
	}}
}

// Method maps a method name to its executable implementation.
func (s *Service) Method(name string) (svc.Executable, error) {
	switch strings.ToLower(name) {
	case "getenv":
		return s.getEnv, nil
	default:
		return nil, svc.NewMethodNotFoundError(name)
	}
}

// getEnv reads environment values for the supplied names. Missing variables are omitted.
func (s *Service) getEnv(_ context.Context, in, out interface{}) error {
	input, ok := in.(*GetEnvInput)
	if !ok {
		return svc.NewInvalidInputError(in)
	}
	output, ok := out.(*GetEnvOutput)
	if !ok {
		return svc.NewInvalidOutputError(out)
	}
	values := map[string]string{}
	for _, name := range input.Names {
		if strings.TrimSpace(name) == "" {
			continue
		}
		if v, ok := os.LookupEnv(name); ok {
			values[name] = v
		}
	}
	output.Values = values
	return nil
}
