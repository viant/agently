package exec

import (
	"context"
	"reflect"
	"strings"

	agentsvc "github.com/viant/agently/genai/service/agent"
	svc "github.com/viant/agently/genai/tool/service"
	"time"
)

const Name = "llm/exec"

// Service exposes an agent runner facade as a tool service.
// Deprecated: prefer llm/agents:run (and llm/agents:list) for unified
// internal/external agent execution and directory. This service remains for
// backward compatibility and may be removed in a future release.
type Service struct {
	agent *agentsvc.Service
}

// New creates a Service bound to the agent runtime.
func New(agent *agentsvc.Service) *Service { return &Service{agent: agent} }

// Name returns the service name.
func (s *Service) Name() string { return Name }

// ToolTimeout suggests a larger timeout for llm/exec service calls which may
// involve long-running agent turns.
func (s *Service) ToolTimeout() time.Duration { return 30 * time.Minute }

// Methods returns the available run methods.
func (s *Service) Methods() svc.Signatures {
	return []svc.Signature{{
		Name:        "run_agent",
		Description: "Run an agent by id with an objective and optional context",
		Input:       reflect.TypeOf(&Input{}),
		Output:      reflect.TypeOf(&Output{}),
	}}
}

// Method resolves a method by name.
func (s *Service) Method(name string) (svc.Executable, error) {
	switch strings.ToLower(name) {
	case "run_agent":
		return s.runAgent, nil
	default:
		return nil, svc.NewMethodNotFoundError(name)
	}
}

// runAgent executes the requested agent with the provided objective.
func (s *Service) runAgent(ctx context.Context, in, out interface{}) error {
	input, ok := in.(*Input)
	if !ok {
		return svc.NewInvalidInputError(in)
	}
	output, ok := out.(*Output)
	if !ok {
		return svc.NewInvalidOutputError(out)
	}
	if s.agent == nil {
		return svc.NewMethodNotFoundError("agent runtime not configured")
	}

	// Translate to agent.Query
	qi := &agentsvc.QueryInput{
		ConversationID:       input.ConversationID,
		ParentConversationID: input.ParentConversationID,
		MessageID:            input.MessageID,
		AgentID:              input.AgentID,
		Query:                input.Objective,
		Context:              input.Context,
	}
	qo := &agentsvc.QueryOutput{}
	if err := s.agent.Query(ctx, qi, qo); err != nil {
		return err
	}
	output.Answer = qo.Content
	return nil
}
