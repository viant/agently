package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	apiconv "github.com/viant/agently/client/conversation"
	elact "github.com/viant/agently/genai/elicitation/action"
	"github.com/viant/agently/genai/memory"
	modelcallctx "github.com/viant/agently/genai/modelcallctx"
	"github.com/viant/agently/genai/prompt"
	"github.com/viant/agently/genai/service/core"
	"github.com/viant/agently/genai/tool"
	"github.com/viant/agently/genai/usage"
	authctx "github.com/viant/agently/internal/auth"
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

	// Bridge auth token from QueryInput.Context when provided (non-HTTP callers).
	if input != nil && input.Context != nil {
		// Accept common keys: authorization (may include "Bearer "), authToken, token, bearer
		if v, ok := input.Context["authorization"].(string); ok && strings.TrimSpace(v) != "" {
			if tok := authctx.ExtractBearer(v); tok != "" {
				ctx = authctx.WithBearer(ctx, tok)
			}
		}
		if v, ok := input.Context["authToken"].(string); ok && strings.TrimSpace(v) != "" {
			ctx = authctx.WithBearer(ctx, v)
		}
		if v, ok := input.Context["token"].(string); ok && strings.TrimSpace(v) != "" {
			ctx = authctx.WithBearer(ctx, v)
		}
		if v, ok := input.Context["bearer"].(string); ok && strings.TrimSpace(v) != "" {
			ctx = authctx.WithBearer(ctx, v)
		}
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
	if err := s.conversation.PatchTurn(ctx, turnRec); err != nil {
		return err
	}
	_, err := s.addMessage(ctx, turn.ConversationID, "user", "", input.Query, turn.ParentMessageID, "")
	if err != nil {
		return fmt.Errorf("failed to add message: %w", err)
	}

	// Persist attachment messages (payload + message linked to user message)
	if len(input.Attachments) > 0 {
		for _, att := range input.Attachments {
			if att == nil || len(att.Data) == 0 {
				continue
			}
			if err = s.addAttachment(ctx, turn, att); err != nil {
				return err
			}
		}
	}
	err = s.runPlanLoop(ctx, input, output)
	status := "succeeded"
	if err != nil {
		status = "failed"
	}
	// Update turn status (and error message on failure) via conversation client
	updTurn := apiconv.NewTurn()
	updTurn.SetId(turn.TurnID)
	updTurn.SetStatus(status)
	if err != nil {
		updTurn.SetErrorMessage(err.Error())
	}
	updateErr := s.conversation.PatchTurn(ctx, updTurn)
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
		if s.conversation != nil {
			mw := convw.Conversation(*w)
			_ = s.conversation.PatchConversations(ctx, (*apiconv.MutableConversation)(&mw))
		}
	}

	// Elicitation and final content persistence are handled inside runPlanLoop now
	output.Usage = agg
	return nil
}

func (s *Service) addAttachment(ctx context.Context, turn memory.TurnMeta, att *prompt.Attachment) error {
	// 1) Create attachment message first (without payload)
	messageID := uuid.New().String()
	msg := apiconv.NewMessage()
	msg.SetId(messageID)
	msg.SetConversationID(turn.ConversationID)
	msg.SetTurnID(turn.TurnID)
	msg.SetParentMessageID(turn.ParentMessageID)
	msg.SetRole("user")
	msg.SetType("control")
	if strings.TrimSpace(att.Name) != "" {
		msg.SetContent(att.Name)
	}
	if err := s.conversation.PatchMessage(ctx, msg); err != nil {
		return fmt.Errorf("failed to persist attachment message: %w", err)
	}

	// 2) Create payload for attachment content
	pid := uuid.New().String()
	payload := apiconv.NewPayload()
	payload.SetId(pid)
	payload.SetKind("model_request")
	payload.SetMimeType(att.MIMEType())
	payload.SetSizeBytes(len(att.Data))
	payload.SetStorage("inline")
	payload.SetInlineBody(att.Data)
	if strings.TrimSpace(att.URI) != "" {
		payload.SetURI(att.URI)
	}
	if err := s.conversation.PatchPayload(ctx, payload); err != nil {
		return fmt.Errorf("failed to persist attachment payload: %w", err)
	}

	link := apiconv.NewMessage()
	link.SetId(messageID)
	link.SetAttachmentPayloadID(pid)
	if err := s.conversation.PatchMessage(ctx, link); err != nil {
		return fmt.Errorf("failed to link attachment payload to message: %w", err)
	}
	return nil
}

func (s *Service) runPlanLoop(ctx context.Context, input *QueryInput, queryOutput *QueryOutput) error {
	var err error

	turn, ok := memory.TurnMetaFromContext(ctx)
	if !ok {
		return fmt.Errorf("failed to get turn meta")
	}

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
		if pErr != nil {
			return pErr
		}
		if aPlan == nil {
			return fmt.Errorf("unable to generate plan")
		}
		queryOutput.Plan = aPlan

		// Handle elicitation inside the loop as a single-turn interaction.
		if aPlan.Elicitation != nil {
			_, status, _, err := s.elicitation.Elicit(ctx, &turn, "assistant", aPlan.Elicitation)
			if err != nil {
				return err
			}
			if elact.Normalize(status) != elact.Accept {
				// User declined/cancelled; finish turn without additional content
				return nil
			}
			// Continue loop with updated binding (which should include payload/user response)
			continue
		}

		// No elicitation: plan either completed with final content or produced tool calls.
		if aPlan.IsEmpty() {
			// Persist final assistant text using the shared message ID
			if strings.TrimSpace(genOutput.Content) != "" {
				modelcallctx.WaitFinish(ctx, 1500*time.Millisecond)
				msgID := memory.ModelMessageIDFromContext(ctx)
				if msgID == "" {
					msgID = genOutput.MessageID
				}
				var parentID, convID string
				if tm, ok := memory.TurnMetaFromContext(ctx); ok {
					parentID = tm.ParentMessageID
					convID = tm.ConversationID
				}
				if _, err := s.addMessage(ctx, convID, "assistant", "", genOutput.Content, msgID, parentID); err != nil {
					return err
				}
			}
			queryOutput.Content = genOutput.Content
			return nil
		}
		// Otherwise, continue loop to allow the orchestrator to perform next step
	}
	return err
}

// waitForElicitation registers a waiter on the elicitation router and optionally
// spawns a local awaiter to resolve the elicitation in interactive environments.
// It returns true when the elicitation was accepted.
// waitForElicitation was inlined into elicitation.Service.Wait

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
	if err := s.conversation.PatchMessage(ctx, m); err != nil {
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
