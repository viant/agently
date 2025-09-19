package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/memory"
	modelcallctx "github.com/viant/agently/genai/modelcallctx"
	"github.com/viant/agently/genai/service/core"
	"github.com/viant/agently/genai/tool"
	"github.com/viant/agently/genai/usage"
	convw "github.com/viant/agently/pkg/agently/conversation/write"
)

// Query executes a query against an agent.
func (s *Service) Query(ctx context.Context, input *QueryInput, output *QueryOutput) error {
	// Ensure conversation exists and reuse stored defaults (agent/model/tools)
	if err := s.ensureConversation(ctx, input); err != nil {
		return err
	}

	// Ensure agent is loaded (may be sourced from conversation when not provided)
	if err := s.ensureAgent(ctx, input); err != nil {
		return err
	}

	if input == nil || input.Agent == nil {
		return fmt.Errorf("invalid input: agent is required")
	}

	if input.EmbeddingModel == "" {
		input.EmbeddingModel = s.defaults.Embedder
	}

	// Conversation already ensured above (fills AgentName/Model/Tools when missing)
	output.ConversationID = input.ConversationID
	s.tryMergePromptIntoContext(input)
	if err := s.updatedConversationContext(ctx, input.ConversationID, input); err != nil {
		return err
	}
	if input.MessageID == "" {
		input.MessageID = uuid.New().String()
	}
	ctx, agg := usage.WithAggregator(ctx)
	turn := memory.TurnMeta{
		ConversationID:  input.ConversationID,
		TurnID:          input.MessageID,
		ParentMessageID: input.MessageID,
	}
	ctx = memory.WithTurnMeta(ctx, turn)
	if len(input.ToolsAllowed) > 0 {
		pol := &tool.Policy{Mode: tool.ModeAuto, AllowList: input.ToolsAllowed}
		ctx = tool.WithPolicy(ctx, pol)
	}

	// Start turn via conversation client
	turnRec := apiconv.NewTurn()
	turnRec.SetId(turn.TurnID)
	turnRec.SetConversationID(turn.ConversationID)
	turnRec.SetStatus("running")
	turnRec.SetCreatedAt(time.Now())
	if err := s.convClient.PatchTurn(ctx, turnRec); err != nil {
		return err
	}
	_, err := s.addMessage(ctx, turn.ConversationID, "user", "", input.Query, turn.ParentMessageID, "")
	if err != nil {
		return fmt.Errorf("failed to add message: %w", err)
	}
	err = s.runPlanLoop(ctx, input, output)
	status := "succeeded"
	if err != nil {
		status = "failed"
	}
	// Update turn status via conversation client
	updTurn := apiconv.NewTurn()
	updTurn.SetId(turn.TurnID)
	updTurn.SetStatus(status)
	updateErr := s.convClient.PatchTurn(ctx, updTurn)
	if err != nil {
		return err
	}
	if updateErr != nil {
		return updateErr
	}
	// Persist/refresh conversation default model with the actually used model this turn
	if strings.TrimSpace(output.Model) != "" {
		w := &convw.Conversation{Has: &convw.ConversationHas{}}
		w.SetId(turn.ConversationID)
		w.SetDefaultModel(output.Model)
		if s.convClient != nil {
			mw := convw.Conversation(*w)
			_ = s.convClient.PatchConversations(ctx, (*apiconv.MutableConversation)(&mw))
		}
	}

	if output.Plan.Elicitation != nil {
		// Wait for model-call persistence so payload ids are ready
		modelcallctx.WaitFinish(ctx, 1500*time.Millisecond)
		if err := s.recordAssistantElicitation(ctx, turn.ConversationID, turn.ParentMessageID, output.Plan.Elicitation); err != nil {
			return err
		}
	} else if strings.TrimSpace(output.Content) != "" {
		// Persist final assistant text using the shared message ID
		modelcallctx.WaitFinish(ctx, 1500*time.Millisecond)
		msgID := memory.ModelMessageIDFromContext(ctx)
		if msgID == "" {
			msgID = output.MessageID
		}
		if _, err := s.addMessage(ctx, turn.ConversationID, "assistant", "", output.Content, msgID, turn.ParentMessageID); err != nil {
			return err
		}
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
			Attachment:     input.Attachments,
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
	// Persist via conversation client
	m := apiconv.NewMessage()
	m.SetId(id)
	m.SetConversationID(convID)
	if turn, ok := memory.TurnMetaFromContext(ctx); ok && strings.TrimSpace(turn.TurnID) != "" {
		m.SetTurnID(turn.TurnID)
	}
	if strings.TrimSpace(parentId) != "" {
		m.SetParentMessageID(parentId)
	}
	if strings.TrimSpace(role) != "" {
		m.SetRole(role)
	}
	// default to text message type
	m.SetType("text")
	if strings.TrimSpace(actor) != "" {
		m.SetCreatedByUserID(actor)
	}
	if strings.TrimSpace(content) != "" {
		m.SetContent(content)
	}
	if err := s.convClient.PatchMessage(ctx, m); err != nil {
		return "", err
	}
	return id, nil
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
