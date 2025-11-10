package sysos

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	mem "github.com/viant/agently/genai/memory"
	svc "github.com/viant/agently/genai/tool/service"
)

// Name identifies this service for MCP routing.
const Name = "system/os"

// Service exposes OS-related helper functions.
type Service struct {
	mu    sync.Mutex
	cache map[string]map[string]cached // convID -> argsKey -> cached
}

type cached struct {
	values   map[string]string
	recorded time.Time
}

// New creates a new Service instance.
func New() *Service { return &Service{cache: map[string]map[string]cached{}} }

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
		Description: "Read environment variables by name. Example: names=['PATH','HOME'].",
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
func (s *Service) getEnv(ctx context.Context, in, out interface{}) error {
	input, ok := in.(*GetEnvInput)
	if !ok {
		return svc.NewInvalidInputError(in)
	}
	output, ok := out.(*GetEnvOutput)
	if !ok {
		return svc.NewInvalidOutputError(out)
	}
	// Guardrails: require at least one non-empty name to avoid pointless tool loops
	// and accidental hammering with empty inputs.
	// Also de-duplicate names within a single call.
	uniq := map[string]struct{}{}
	for _, raw := range input.Names {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		uniq[name] = struct{}{}
	}
	if len(uniq) == 0 {
		return fmt.Errorf("names is required; include at least one environment variable name")
	}

	// Compose a stable key from sorted unique names for per-conversation memoization
	names := make([]string, 0, len(uniq))
	for n := range uniq {
		names = append(names, n)
	}
	sort.Strings(names)
	argsKey := strings.Join(names, ",")
	convID := mem.ConversationIDFromContext(ctx)

	// Return recent cached values when identical request repeats quickly
	const cooldown = 5 * time.Second
	s.mu.Lock()
	if s.cache[convID] != nil {
		if c, ok := s.cache[convID][argsKey]; ok && time.Since(c.recorded) <= cooldown {
			output.Values = c.values
			s.mu.Unlock()
			return nil
		}
	}
	s.mu.Unlock()

	values := map[string]string{}
	for _, name := range names {
		if v, ok := os.LookupEnv(name); ok {
			values[name] = v
		}
	}
	output.Values = values

	// Cache the result for a short window to avoid tight loops
	s.mu.Lock()
	if s.cache[convID] == nil {
		s.cache[convID] = map[string]cached{}
	}
	s.cache[convID][argsKey] = cached{values: values, recorded: time.Now()}
	s.mu.Unlock()
	return nil
}
