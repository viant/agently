package agent

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	apiconv "github.com/viant/agently/client/conversation"
	elact "github.com/viant/agently/genai/elicitation/action"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/memory"
	modelcallctx "github.com/viant/agently/genai/modelcallctx"
	"github.com/viant/agently/genai/prompt"
	"github.com/viant/agently/genai/service/core"
	executil "github.com/viant/agently/genai/service/shared/executil"
	"github.com/viant/agently/genai/tool"
	"github.com/viant/agently/genai/usage"
	authctx "github.com/viant/agently/internal/auth"
	convw "github.com/viant/agently/pkg/agently/conversation/write"
)

// executeChains filters, evaluates and dispatches chains declared on the parent agent.

// Query executes a query against an agent.
func (s *Service) Query(ctx context.Context, input *QueryInput, output *QueryOutput) error {
	if err := s.ensureEnvironment(ctx, input); err != nil {
		return err
	}
	if input == nil || input.Agent == nil {
		return fmt.Errorf("invalid input: agent is required")
	}

	// Bridge auth token from QueryInput.Context when provided (non-HTTP callers).
	ctx = s.bindAuthFromInputContext(ctx, input)

	// Install a warnings collector in context for this turn.
	ctx, _ = withWarnings(ctx)

	// Conversation already ensured above (fills AgentID/Model/Tools when missing)
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
		Assistant:       input.Agent.ID,
		ConversationID:  input.ConversationID,
		TurnID:          input.MessageID,
		ParentMessageID: input.MessageID,
	}
	ctx = memory.WithTurnMeta(ctx, turn)

	// Establish authoritative cancel and register it if available
	var cancel func()
	ctx, cancel = s.registerTurnCancel(ctx, turn)
	defer cancel()
	if len(input.ToolsAllowed) > 0 {
		pol := &tool.Policy{Mode: tool.ModeAuto, AllowList: input.ToolsAllowed}
		ctx = tool.WithPolicy(ctx, pol)
	}

	// Start turn and persist initial user message
	if err := s.startTurn(ctx, turn); err != nil {
		return err
	}
	if err := s.addUserMessage(ctx, &turn, input); err != nil {
		return err
	}

	// Persist attachments if any
	if err := s.processAttachments(ctx, turn, input); err != nil {
		return err
	}
	// No pre-execution elicitation. Templates can instruct LLM to elicit details
	// using binding.Elicitation. Orchestrator handles assistant-originated elicitations.
	// Apply workspace-configured tool timeout to context, if set.
	if s.defaults != nil && s.defaults.ToolCallTimeoutSec > 0 {
		d := time.Duration(s.defaults.ToolCallTimeoutSec) * time.Second
		ctx = executil.WithToolTimeout(ctx, d)
	}
	status, err := s.runPlanAndStatus(ctx, input, output)

	if err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("execution of query function failed (context canceled): %w", err)
	}
	if err != nil {
		return fmt.Errorf("execution of query function failed: %w", err)
	}

	if err := s.finalizeTurn(ctx, turn, status, err); err != nil {
		return err
	}
	// Persist/refresh conversation default model with the actually used model this turn
	_ = s.updateDefaultModel(ctx, turn, output)

	conv, err := s.fetchConversationWithRetry(ctx, input.ConversationID, apiconv.WithIncludeToolCall(true))
	if err != nil {
		return fmt.Errorf("cannot get conversation: %w", err)
	}
	if conv == nil {
		return fmt.Errorf("cannot get conversation: not found: %s", strings.TrimSpace(input.ConversationID))
	}
	// Elicitation and final content persistence are handled inside runPlanLoop now
	output.Usage = agg
	// Expose any collected warnings on query output.
	if ws := warningsFrom(ctx); len(ws) > 0 {
		output.Warnings = ws
	}
	if err := s.executeChainsAfter(ctx, input, output, turn, conv, status); err != nil {
		return err
	}
	if conv.HasConversationParent() || conv.ScheduleId != nil {
		return nil
	}
	err = s.summarizeIfNeeded(ctx, input, conv)
	if err != nil {
		return fmt.Errorf("failed summarizing: %w", err)
	}
	return nil
}

// loopControls captures continuation flags from Context.chain.loop
func (s *Service) addAttachment(ctx context.Context, turn memory.TurnMeta, att *prompt.Attachment) error {
	// 1) Create attachment message first (without payload)
	messageID := uuid.New().String()
	opts := []apiconv.MessageOption{
		apiconv.WithId(messageID),
		apiconv.WithRole("user"),
		apiconv.WithType("control"),
	}
	if strings.TrimSpace(att.Name) != "" {
		opts = append(opts, apiconv.WithContent(att.Name))
	}
	if _, err := apiconv.AddMessage(ctx, s.conversation, &turn, opts...); err != nil {
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
		if modelSelection.Options == nil {
			modelSelection.Options = &llm.Options{}
		}
		queryOutput.Model = modelSelection.Model
		queryOutput.Agent = input.Agent
		genInput := &core.GenerateInput{
			Prompt:         input.Agent.Prompt,
			SystemPrompt:   input.Agent.SystemPrompt,
			Binding:        binding,
			ModelSelection: modelSelection,
		}
		// Attribute participants for multi-user/agent naming in LLM messages
		genInput.UserID = strings.TrimSpace(input.UserId)
		if input.Agent != nil {
			genInput.AgentID = strings.TrimSpace(input.Agent.ID)
		}
		genInput.Options.Mode = "plan"
		EnsureGenerateOptions(ctx, genInput, input.Agent)
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
			ectx := ctx
			var cancel func()
			if s.defaults != nil && s.defaults.ElicitationTimeoutSec > 0 {
				ectx, cancel = context.WithTimeout(ctx, time.Duration(s.defaults.ElicitationTimeoutSec)*time.Second)
				defer cancel()
			}
			_, status, _, err := s.elicitation.Elicit(ectx, &turn, "assistant", aPlan.Elicitation)
			if err != nil {
				// If timed out or canceled, auto-decline to avoid getting stuck
				if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
					_ = s.elicitation.Resolve(context.Background(), turn.ConversationID, aPlan.Elicitation.ElicitationId, "decline", nil, "timeout")
					return nil
				}
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
				// Attribute assistant message to the agent ID for history and UI display
				actor := input.Actor()
				if _, err := s.addMessage(ctx, &turn, "assistant", actor, genOutput.Content, "plan", msgID); err != nil {
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

func (s *Service) addMessage(ctx context.Context, turn *memory.TurnMeta, role, actor, content, mode, id string) (string, error) {
	opts := []apiconv.MessageOption{
		apiconv.WithRole(role),
		apiconv.WithCreatedByUserID(actor),
		apiconv.WithContent(content),
		apiconv.WithMode(mode),
	}
	if strings.TrimSpace(id) != "" {
		opts = append(opts, apiconv.WithId(id))
	}
	msg, err := apiconv.AddMessage(ctx, s.conversation, turn, opts...)
	if err != nil {
		return "", fmt.Errorf("failed to add message: %w", err)
	}
	return msg.Id, nil
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

// ensureEnvironment ensures conversation and agent are initialized and sets defaults.
func (s *Service) ensureEnvironment(ctx context.Context, input *QueryInput) error {
	if err := s.ensureConversation(ctx, input); err != nil {
		return err
	}
	if err := s.ensureAgent(ctx, input); err != nil {
		return err
	}
	if input.EmbeddingModel == "" {
		input.EmbeddingModel = s.defaults.Embedder
	}
	return nil
}

// bindAuthFromInputContext extracts bearer tokens from input.Context and attaches to ctx.
func (s *Service) bindAuthFromInputContext(ctx context.Context, input *QueryInput) context.Context {
	if input == nil || input.Context == nil {
		return ctx
	}
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
	return ctx
}

func (s *Service) buildTurnMeta(input *QueryInput) memory.TurnMeta {
	return memory.TurnMeta{Assistant: input.Agent.ID, ConversationID: input.ConversationID, TurnID: input.MessageID, ParentMessageID: input.MessageID}
}

// registerTurnCancel returns a derived context and a deferred cancel wrapper that patches status=canceled.
func (s *Service) registerTurnCancel(ctx context.Context, turn memory.TurnMeta) (context.Context, func()) {
	ctx, cancel := context.WithCancel(ctx)
	wrappedCancel := func() {
		cancel()
		if s.conversation != nil {
			upd := apiconv.NewTurn()
			upd.SetId(turn.TurnID)
			upd.SetStatus("canceled")
			_ = s.conversation.PatchTurn(context.Background(), upd)
		}
	}
	if s.cancelReg != nil {
		s.cancelReg.Register(turn.ConversationID, turn.TurnID, wrappedCancel)
		return ctx, func() { s.cancelReg.Complete(turn.ConversationID, turn.TurnID, wrappedCancel) }
	}
	return ctx, wrappedCancel
}

func (s *Service) startTurn(ctx context.Context, turn memory.TurnMeta) error {
	rec := apiconv.NewTurn()
	rec.SetId(turn.TurnID)
	rec.SetConversationID(turn.ConversationID)
	rec.SetStatus("running")
	rec.SetCreatedAt(time.Now())
	return s.conversation.PatchTurn(ctx, rec)
}

func (s *Service) addUserMessage(ctx context.Context, turn *memory.TurnMeta, input *QueryInput) error {
	_, err := s.addMessage(ctx, turn, "user", input.UserId, input.Query, "task", turn.TurnID)
	if err != nil {
		return fmt.Errorf("failed to add message: %w", err)
	}
	return nil
}

func (s *Service) processAttachments(ctx context.Context, turn memory.TurnMeta, input *QueryInput) error {
	if len(input.Attachments) == 0 {
		return nil
	}
	modelName := ""
	if input.ModelOverride != "" {
		modelName = input.ModelOverride
	} else if input.Agent != nil {
		modelName = input.Agent.Model
	}
	model, _ := s.llm.ModelFinder().Find(ctx, modelName)
	var limit int64
	if input.Agent != nil && input.Agent.Attachment != nil && input.Agent.Attachment.LimitBytes > 0 {
		limit = input.Agent.Attachment.LimitBytes
	} else {
		limit = s.llm.ProviderAttachmentLimit(model)
	}
	used := s.llm.AttachmentUsage(turn.ConversationID)
	var appended int64
	for _, att := range input.Attachments {
		if att == nil || len(att.Data) == 0 {
			continue
		}
		if limit > 0 {
			remain := limit - used - appended
			size := int64(len(att.Data))
			if remain <= 0 || size > remain {
				name := strings.TrimSpace(att.Name)
				if name == "" {
					name = "(unnamed)"
				}
				limMB := float64(limit) / (1024.0 * 1024.0)
				usedMB := float64(used+appended) / (1024.0 * 1024.0)
				curMB := float64(size) / (1024.0 * 1024.0)
				return fmt.Errorf("attachments exceed agent cap: limit %.3f MB, used %.3f MB, current (%s) %.3f MB", limMB, usedMB, name, curMB)
			}
		}
		if err := s.addAttachment(ctx, turn, att); err != nil {
			return err
		}
		appended += int64(len(att.Data))
	}
	if appended > 0 {
		s.llm.SetAttachmentUsage(turn.ConversationID, used+appended)
		_ = s.updateAttachmentUsageMetadata(ctx, turn.ConversationID, used+appended)
	}
	return nil
}

func (s *Service) runPlanAndStatus(ctx context.Context, input *QueryInput, output *QueryOutput) (string, error) {
	if err := s.runPlanLoop(ctx, input, output); err != nil {
		if errors.Is(err, context.Canceled) {
			return "canceled", err
		}
		return "failed", err
	}
	return "succeeded", nil
}

func (s *Service) finalizeTurn(ctx context.Context, turn memory.TurnMeta, status string, runErr error) error {
	var emsg string
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		emsg = runErr.Error()
	}
	patchCtx := ctx
	if status == "canceled" {
		patchCtx = context.Background()
	}
	upd := apiconv.NewTurn()
	upd.SetId(turn.TurnID)
	upd.SetStatus(status)
	if emsg != "" {
		upd.SetErrorMessage(emsg)
	}

	if err := s.conversation.PatchTurn(patchCtx, upd); runErr != nil {
		return runErr
	} else if err != nil {
		return err
	}
	if err := s.conversation.PatchConversations(ctx, convw.NewConversationStatus(turn.ConversationID, status)); err != nil {
		return fmt.Errorf("failed to update conversation: %w", err)
	}
	return nil
}

func (s *Service) updateDefaultModel(ctx context.Context, turn memory.TurnMeta, output *QueryOutput) error {
	if strings.TrimSpace(output.Model) == "" {
		return nil
	}
	w := &convw.Conversation{Has: &convw.ConversationHas{}}
	w.SetId(turn.ConversationID)
	w.SetDefaultModel(output.Model)
	if s.conversation != nil {
		mw := convw.Conversation(*w)
		_ = s.conversation.PatchConversations(ctx, (*apiconv.MutableConversation)(&mw))
	}
	return nil
}

func (s *Service) executeChainsAfter(ctx context.Context, input *QueryInput, output *QueryOutput, turn memory.TurnMeta, conv *apiconv.Conversation, status string) error {
	cc := NewChainContext(input, output, &turn)
	cc.Conversation = conv
	return s.executeChains(ctx, cc, status)
}
