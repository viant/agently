package agent

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/agent/plan"
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

	// Conversation already ensured above (fills AgentID/Model/Tool when missing)
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

	// Start turn and persist initial user message. Prefer using the
	// expanded user prompt (via llm/core:expandUserPrompt) so the
	// conversation stores a single, canonical task for this turn.
	if err := s.startTurn(ctx, turn); err != nil {
		return err
	}
	// Best-effort expansion of the user prompt only on the very first turn of a conversation.
	rawUserContent := input.Query
	content := strings.TrimSpace(input.Query)
	if input.IsNewConversation && s.llm != nil && input.Agent != nil {
		b, berr := s.BuildBinding(ctx, input)
		if berr == nil {
			var expOut core.ExpandUserPromptOutput
			expIn := &core.ExpandUserPromptInput{Prompt: input.Agent.Prompt, Binding: b}
			if err := s.llm.ExpandUserPrompt(ctx, expIn, &expOut); err == nil && strings.TrimSpace(expOut.ExpandedUserPrompt) != "" {
				content = expOut.ExpandedUserPrompt
			}
		}
	}
	if err := s.addUserMessage(ctx, &turn, input.UserId, content, rawUserContent); err != nil {
		return err
	}

	// Persist attachments if any. Once persisted into history, avoid also
	// sending them as task-scoped attachments to prevent duplicate media in
	// the provider request payload.
	if err := s.processAttachments(ctx, turn, input); err != nil {
		return err
	}

	// TODO delete if not needed
	//if len(input.Attachments) > 0 {
	//    input.Attachments = nil
	//}

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

	parentMsgID := strings.TrimSpace(turn.ParentMessageID)
	if parentMsgID == "" {
		parentMsgID = strings.TrimSpace(turn.TurnID)
	}

	name := strings.TrimSpace(att.Name)
	if name == "" && strings.TrimSpace(att.URI) != "" {
		name = path.Base(strings.TrimSpace(att.URI))
	}
	if name == "" {
		name = "(attachment)"
	}

	_, err := apiconv.AddMessage(ctx, s.conversation, &turn,
		apiconv.WithRole("user"),
		apiconv.WithType("control"),
		apiconv.WithParentMessageID(parentMsgID),
		apiconv.WithContent(name),
		apiconv.WithAttachmentPayloadID(pid),
	)
	if err != nil {
		return fmt.Errorf("failed to persist attachment message: %w", err)
	}
	return nil
}

func (s *Service) runPlanLoop(ctx context.Context, input *QueryInput, queryOutput *QueryOutput) error {
	var err error
	iter := 0
	// resolvedModel tracks the first model selected (either via explicit
	// override or matcher-based preferences) for this Query turn. Once set,
	// subsequent iterations within the same turn stick to this model instead
	// of re-evaluating preferences, to keep provider/model stable.
	var resolvedModel string

	turn, ok := memory.TurnMetaFromContext(ctx)
	if !ok {
		return fmt.Errorf("failed to get turn meta")
	}

	input.RequestTime = time.Now()
	for {
		iter++
		binding, bErr := s.BuildBinding(ctx, input)
		if bErr != nil {
			return bErr
		}
		// Context keys snapshot
		keys := []string{}
		for k := range binding.Context {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		modelSelection := input.Agent.ModelSelection
		// Once a model has been resolved earlier in this turn (either via
		// explicit override or matcher-based preferences), stick to it for
		// the rest of the turn to avoid re-evaluating preferences and
		// changing models mid‑execution.
		if strings.TrimSpace(resolvedModel) != "" && strings.TrimSpace(input.ModelOverride) == "" {
			modelSelection.Model = resolvedModel
			modelSelection.Preferences = nil
		} else {
			// ModelOverride, when present, always wins for this turn.
			if input.ModelOverride != "" {
				modelSelection.Model = input.ModelOverride
			} else if input.ModelPreferences != nil {
				// When the caller supplies per-turn model preferences without an
				// explicit override, clear the configured model so that
				// GenerateInput.MatchModelIfNeeded can pick the best candidate
				// using the workspace model matcher. This allows callers of
				// llm/agents:run (and direct Query) to influence model choice
				// beyond the agent's static modelRef.
				modelSelection.Model = ""
			}
			if input.ModelPreferences != nil {
				modelSelection.Preferences = input.ModelPreferences
			}
			// Gatekeeper: set allowed reductions from agent config; when providers empty, derive from agent default model provider.
			if input.Agent != nil {
				modelSelection.AllowedModels = nil
				if len(input.Agent.AllowedModels) > 0 {
					for _, id := range input.Agent.AllowedModels {
						if v := strings.TrimSpace(id); v != "" {
							modelSelection.AllowedModels = append(modelSelection.AllowedModels, v)
						}
					}
				}
				modelSelection.AllowedProviders = nil
				if len(input.Agent.AllowedProviders) > 0 {
					for _, p := range input.Agent.AllowedProviders {
						if v := strings.TrimSpace(p); v != "" {
							modelSelection.AllowedProviders = append(modelSelection.AllowedProviders, v)
						}
					}
				} else if prov := deriveProviderFromModelRef(input.Agent.Model); prov != "" {
					modelSelection.AllowedProviders = []string{prov}
				}
			}
		}
		// Keep allowed reductions across iterations when available.
		if input.Agent != nil && len(modelSelection.AllowedProviders) == 0 && len(modelSelection.AllowedModels) == 0 {
			if prov := deriveProviderFromModelRef(input.Agent.Model); prov != "" {
				modelSelection.AllowedProviders = []string{prov}
			}
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
		// The user task for this turn has already been expanded and
		// persisted as the latest user message in history; avoid adding
		// another synthetic user message in History.Current.
		genInput.UserPromptAlreadyInHistory = true
		// Attribute participants for multi-user/agent naming in LLM messages
		genInput.UserID = strings.TrimSpace(input.UserId)
		if input.Agent != nil {
			genInput.AgentID = strings.TrimSpace(input.Agent.ID)
		}
		genInput.Options.Mode = "plan"
		EnsureGenerateOptions(ctx, genInput, input.Agent)
		// Apply per-turn override for reasoning effort when requested
		if input.ReasoningEffort != nil {
			if v := strings.TrimSpace(*input.ReasoningEffort); v != "" {
				if genInput.ModelSelection.Options.Reasoning == nil {
					genInput.ModelSelection.Options.Reasoning = &llm.Reasoning{}
				}
				genInput.ModelSelection.Options.Reasoning.Effort = v
			}
		}
		genOutput := &core.GenerateOutput{}
		aPlan, pErr := s.orchestrator.Run(ctx, genInput, genOutput)
		if pErr != nil {
			return pErr
		}
		if aPlan == nil {
			return fmt.Errorf("unable to generate plan")
		}
		// Capture the first resolved model for this turn and stick to it on
		// subsequent iterations when preferences are used.
		if strings.TrimSpace(resolvedModel) == "" && genInput != nil {
			if m := strings.TrimSpace(genInput.ModelSelection.Model); m != "" {
				resolvedModel = m
			}
		}
		queryOutput.Plan = aPlan

		// Detect duplicated tool steps in the plan and attach warnings to the turn context.
		s.warnOnDuplicateSteps(ctx, aPlan)

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
				if _, err := s.addMessage(ctx, &turn, "assistant", actor, genOutput.Content, nil, "plan", msgID); err != nil {
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

// warnOnDuplicateSteps scans a plan for duplicate tool steps (same name and canonicalised args)
// and appends a warning to the per-turn warnings collector. It does not modify the plan.
func (s *Service) warnOnDuplicateSteps(ctx context.Context, p *plan.Plan) {
	if p == nil || len(p.Steps) == 0 {
		return
	}
	type key struct{ Name, Args string }
	seen := map[key]struct{}{}
	for _, st := range p.Steps {
		if strings.TrimSpace(st.Type) != "tool" {
			continue
		}
		k := key{Name: strings.TrimSpace(st.Name), Args: canonicalArgsForWarning(st.Args)}
		if _, ok := seen[k]; ok {
			appendWarning(ctx, fmt.Sprintf("duplicate tool step detected: %s %s", k.Name, k.Args))
			continue
		}
		seen[k] = struct{}{}
	}
}

// canonicalArgsForWarning returns a deterministic JSON string for args to detect duplicates.
// This local copy avoids importing internal packages; it is intentionally minimal and only
// used for warnings (not for execution decisions).
func canonicalArgsForWarning(args map[string]interface{}) string {
	if len(args) == 0 {
		return "{}"
	}
	var canonicalize func(v interface{}) interface{}
	canonicalize = func(v interface{}) interface{} {
		switch tv := v.(type) {
		case map[string]interface{}:
			if len(tv) == 0 {
				return map[string]interface{}{}
			}
			keys := make([]string, 0, len(tv))
			for k := range tv {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			out := make(map[string]interface{}, len(tv))
			for _, k := range keys {
				out[k] = canonicalize(tv[k])
			}
			return out
		case []interface{}:
			arr := make([]interface{}, len(tv))
			for i, el := range tv {
				arr[i] = canonicalize(el)
			}
			return arr
		default:
			return tv
		}
	}
	canon := canonicalize(args)
	b, _ := json.Marshal(canon)
	return string(b)
}

// addPreferenceHintsFromAgent appends model preference hints derived from the
// agent configuration. When AllowedModels are set, they are preferred. When
// AllowedProviders are set, they are used. When both are empty, this falls back
// to the current agent provider (derived from modelRef) as an allowed provider.
// NOTE: AllowedProviders/AllowedModels now act as gatekeepers (candidate reducer)
// and must not be written into hints. Selection reduction is handled in
// core.GenerateInput.MatchModelIfNeeded via ReducingMatcher.

// deriveProviderFromModelRef returns the provider name from a modelRef in the
// common form "provider_model". Returns empty string when it cannot be derived.
func deriveProviderFromModelRef(modelRef string) string {
	v := strings.TrimSpace(modelRef)
	if v == "" {
		return ""
	}
	// Heuristic: take the prefix before the first underscore as provider id.
	if idx := strings.IndexRune(v, '_'); idx > 0 {
		return strings.TrimSpace(v[:idx])
	}
	return ""
}

// waitForElicitation registers a waiter on the elicitation router and optionally
// spawns a local awaiter to resolve the elicitation in interactive environments.
// It returns true when the elicitation was accepted.
// waitForElicitation was inlined into elicitation.Service.Wait

func (s *Service) addMessage(ctx context.Context, turn *memory.TurnMeta, role, actor, content string, raw *string, mode, id string) (string, error) {
	opts := []apiconv.MessageOption{
		apiconv.WithRole(role),
		apiconv.WithCreatedByUserID(actor),
		apiconv.WithContent(content),
		apiconv.WithMode(mode),
	}
	if raw != nil {
		trimmed := strings.TrimSpace(*raw)
		if trimmed != "" {
			val := *raw
			opts = append(opts, apiconv.WithRawContent(val))
		}
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

func (s *Service) addUserMessage(ctx context.Context, turn *memory.TurnMeta, userID, content, raw string) error {
	var rawPtr *string
	if strings.TrimSpace(raw) != "" {
		rawCopy := raw
		rawPtr = &rawCopy
	}
	_, err := s.addMessage(ctx, turn, "user", userID, content, rawPtr, "task", turn.TurnID)
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
