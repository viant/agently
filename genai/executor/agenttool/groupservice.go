package agenttool

import (
	"context"
	"fmt"
	"reflect"

	"github.com/viant/agently/genai/agent"
	mcpsvc "github.com/viant/fluxor-mcp/mcp"
	"github.com/viant/fluxor/model/types"
)

// RunInput and RunOutput already defined in runservice.go

// GroupService groups multiple agents under one service; each agent is exposed
// as an individual method.
type GroupService struct {
	name   string
	orch   *mcpsvc.Service
	agents map[string]*agent.Agent // method -> agent
}

func NewGroupService(name string, orch *mcpsvc.Service) *GroupService {
	return &GroupService{
		name:   name,
		orch:   orch,
		agents: map[string]*agent.Agent{},
	}
}

func (gs *GroupService) Name() string { return gs.name }

func (gs *GroupService) Methods() types.Signatures {
	sigs := make(types.Signatures, 0, len(gs.agents))
	for m, ag := range gs.agents {
		sigs = append(sigs, types.Signature{
			Name:        m,
			Description: fmt.Sprintf("Executes the \"%s\" agent", ag.Name),
			Input:       reflect.TypeOf(&RunInput{}),
			Output:      reflect.TypeOf(&RunOutput{}),
		})
	}
	return sigs
}

// Method returns executable for given method.
func (gs *GroupService) Method(name string) (types.Executable, error) {
	ag, ok := gs.agents[name]
	if !ok {
		return nil, types.NewMethodNotFoundError(name)
	}
	return gs.runnerFor(ag), nil
}

func (gs *GroupService) runnerFor(a *agent.Agent) types.Executable {
	return func(ctx context.Context, in, out interface{}) error {
		input, ok := in.(*RunInput)
		if !ok {
			return types.NewInvalidInputError(in)
		}
		output, ok := out.(*RunOutput)
		if !ok {
			return types.NewInvalidOutputError(out)
		}

		if gs.orch == nil {
			return fmt.Errorf("orchestration service is nil")
		}

		args := map[string]interface{}{
			"agentId":   a.ID,
			"objective": input.Objective,
			"context":   input.Context,
		}

		res, err := gs.orch.ExecuteTool(ctx, "llm/exec:run_agent", args, 0)
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
		output.AgentID = a.ID
		return nil
	}
}

// Add links an agent to the specified method.
func (gs *GroupService) Add(method string, ag *agent.Agent) {
	gs.agents[method] = ag
}
