package agenttool

import (
	"context"
	"fmt"
	"reflect"

	"github.com/viant/agently/genai/agent"
	mcpsvc "github.com/viant/fluxor-mcp/mcp"
	"github.com/viant/fluxor/model/types"
)

// RunInput is the parameter object accepted by every agent-runner method.
type RunInput struct {
	Objective string                 `json:"objective,omitempty"`
	Context   map[string]interface{} `json:"context,omitempty"`
}

// RunOutput defines the minimal structure returned to callers.
type RunOutput struct {
	Answer  string      `json:"answer,omitempty"`
	Trace   interface{} `json:"trace,omitempty"`
	AgentID string      `json:"agentId,omitempty"`
}

// agentRunService is a thin Fluxor service facade around an Agent.
type agentRunService struct {
	name   string
	method string
	agent  *agent.Agent
	orch   *mcpsvc.Service
}

// Name implements types.Service.
func (s *agentRunService) Name() string { return s.name }

// Methods implements types.Service.
func (s *agentRunService) Methods() types.Signatures {
	return types.Signatures{{
		Name:        s.method,
		Description: fmt.Sprintf("Executes the \"%s\" agent", s.agent.Name),
		Input:       reflect.TypeOf(&RunInput{}),
		Output:      reflect.TypeOf(&RunOutput{}),
	}}
}

// Method implements types.Service.
func (s *agentRunService) Method(name string) (types.Executable, error) {
	if name != s.method {
		return nil, types.NewMethodNotFoundError(name)
	}
	return s.run, nil
}

func (s *agentRunService) run(ctx context.Context, in, out interface{}) error {
	input, ok := in.(*RunInput)
	if !ok {
		return types.NewInvalidInputError(in)
	}
	output, ok := out.(*RunOutput)
	if !ok {
		return types.NewInvalidOutputError(out)
	}

	if s.orch == nil {
		return fmt.Errorf("orchestration service is nil")
	}

	args := map[string]interface{}{
		"agentId":   s.agent.ID,
		"objective": input.Objective,
		"context":   input.Context,
	}

	res, err := s.orch.ExecuteTool(ctx, "llm/exec:run_agent", args, 0)
	if err != nil {
		return err
	}

	switch v := res.(type) {
	case string:
		output.Answer = v
	default:
		if m, ok := v.(map[string]interface{}); ok {
			if ans, ok2 := m["answer"].(string); ok2 {
				output.Answer = ans
			}
			output.Trace = m
		}
	}
	output.AgentID = s.agent.ID
	return nil
}

// NewService constructs and registers an agentRunService on the supplied
// Actions registry.
func NewService(svcName, method string, ag *agent.Agent, orch *mcpsvc.Service) types.Service {
	return &agentRunService{name: svcName, method: method, agent: ag, orch: orch}
}
