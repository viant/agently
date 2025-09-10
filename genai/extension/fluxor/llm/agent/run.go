package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	core "github.com/viant/agently/genai/extension/fluxor/llm/core"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/tool"
	"github.com/viant/agently/genai/usage"
)

// Query executes a query against an agent.
func (s *Service) Query(ctx context.Context, input *QueryInput, output *QueryOutput) error {

	// 4. Ensure agent is loaded
	if err := s.ensureAgent(ctx, input); err != nil {
		return err
	}

	if input == nil || input.Agent == nil {
		return fmt.Errorf("invalid input: agent is required")
	}

	if err := s.ensureConversation(ctx, input); err != nil {
		return err
	}

	s.tryMergePromptIntoContext(input)
	if err := s.updatedConversationContext(ctx, input.ConversationID, input); err != nil {
		return err
	}

	ctx, agg := usage.WithAggregator(ctx)
	turn := memory.TurnMeta{
		ConversationID:  input.ConversationID,
		TurnID:          uuid.New().String(),
		ParentMessageID: uuid.New().String(),
	}
	ctx = memory.WithTurnMeta(ctx, turn)
	if len(input.ToolsAllowed) > 0 {
		pol := &tool.Policy{Mode: tool.ModeAuto, AllowList: input.ToolsAllowed}
		ctx = tool.WithPolicy(ctx, pol)
	}

	s.recorder.StartTurn(ctx, turn.ConversationID, turn.TurnID, time.Now())
	_, err := s.addMessage(ctx, turn.ConversationID, "user", "", input.Query, turn.ParentMessageID, "")
	if err != nil {
		return fmt.Errorf("failed to add message: %w", err)
	}
	err = s.runPlanLoop(ctx, input, output)
	status := "succeeded"
	if err != nil {
		status = "failed"
	}
	s.recorder.UpdateTurn(ctx, turn.TurnID, status)
	if err != nil {
		return err
	}
	if output.Plan.Elicitation != nil {
		s.recordAssistantElicitation(ctx, turn.TurnID, turn.ParentMessageID, output.Plan.Elicitation)
	}
	output.Usage = agg
	return nil
}

func (s *Service) runPlanLoop(ctx context.Context, input *QueryInput, queryOutput *QueryOutput) error {
	var err error
	for {
		binding, bErr := s.BuildBinding(ctx, input)
		if bErr != nil {
			return bErr
		}
		modelSelection := input.Agent.ModelSelection
		if input.ModelOverride != "" {
			modelSelection.Model = input.ModelOverride
		}
		queryOutput.Model = modelSelection.Model
		queryOutput.Agent = input.Agent
		genInput := &core.GenerateInput{
			Prompt:         input.Agent.Prompt,
			SystemPrompt:   input.Agent.SystemPrompt,
			Binding:        binding,
			ModelSelection: modelSelection,
		}
		genOutput := &core.GenerateOutput{}
		aPlan, pErr := s.orchestrator.Run(ctx, genInput, genOutput)
		if aPlan != nil {
			queryOutput.Plan = aPlan
			if aPlan.Elicitation != nil {
				queryOutput.Elicitation = aPlan.Elicitation
				break
			}
			if aPlan.IsEmpty() {
				queryOutput.Content = genOutput.Content
				break
			}
		}
		if pErr != nil {
			err = pErr
			break
		}
		if aPlan == nil {
			err = fmt.Errorf("unable to generate plan")
			break
		}
	}
	return err
}

func (s *Service) addMessage(ctx context.Context, convID, role, actor, content, id string, parentId string) (string, error) {
	if strings.TrimSpace(content) == "" {
		return "", nil
	}
	if id == "" {
		id = uuid.New().String()
	}
	msg := memory.Message{ID: id, ParentID: parentId, Role: role, Actor: actor, Content: content, ConversationID: convID}
	if s.recorder != nil {
		s.recorder.RecordMessage(ctx, msg)
	}
	return msg.ID, nil
}

// mergeInlineJSONIntoContext copies JSON object fields from qi.Query into qi.Context (non-destructive).
func (s *Service) tryMergePromptIntoContext(input *QueryInput) {
	if input == nil || strings.TrimSpace(input.Query) == "" {
		return
	}
	var tmp map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(input.Query)), &tmp); err == nil && len(tmp) > 0 {
		if input.Context == nil {
			input.Context = map[string]interface{}{}
		}
		for k, v := range tmp {
			if _, exists := input.Context[k]; !exists {
				input.Context[k] = v
			}
		}
	}
}
