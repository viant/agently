package orchestrator

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	apiconv "github.com/viant/agently/client/conversation"
	agentmdl "github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/llm/provider/base"
	"github.com/viant/agently/genai/memory"
	core2 "github.com/viant/agently/genai/service/core"
	"github.com/viant/agently/genai/service/core/stream"
	executil "github.com/viant/agently/genai/service/shared/executil"
	"github.com/viant/agently/genai/tool"
	"github.com/viant/agently/shared"
)

//go:embed free_token_prompt.md
var freeTokenPrompt string

type Service struct {
	llm        *core2.Service
	registry   tool.Registry
	convClient apiconv.Client
	// Finder for agent metadata (prompts, model, prefs) to mirror agent-run plan input
	agentFinder agentmdl.Finder
	// Optional builder to produce a GenerateInput identical to agent.runPlanLoop,
	// with the exception that the user query is provided as `instruction`.
	buildPlanInput func(ctx context.Context, conv *apiconv.Conversation, instruction string) (*core2.GenerateInput, error)
}

// ctxKeyLimitRecoveryAttempted guards one-shot presentation of the context-limit guidance within a single Run invocation.
type ctxKeyPresentedType int

const ctxKeyLimitRecoveryAttempted ctxKeyPresentedType = 1

func (s *Service) Run(ctx context.Context, genInput *core2.GenerateInput, genOutput *core2.GenerateOutput) (*plan.Plan, error) {
	aPlan := plan.New()

	var wg sync.WaitGroup
	nextStepIdx := 0
	// Binding registry to current conversation (if any) so tool.Execute receives ctx with convID.
	reg := tool.WithConversation(s.registry, memory.ConversationIDFromContext(ctx))
	// Do not create child cancels here; errors must not cancel context.
	streamId, stepErrCh := s.registerStreamPlannerHandler(ctx, reg, aPlan, &wg, &nextStepIdx, genOutput)
	canStream, err := s.canStream(ctx, genInput)
	if err != nil {
		return nil, fmt.Errorf("failed to check if model can stream: %w", err)
	}
	if canStream {
		cleanup, err := s.llm.Stream(ctx, &core2.StreamInput{StreamID: streamId, GenerateInput: genInput}, &core2.StreamOutput{})
		defer cleanup()
		if err != nil {
			if errors.Is(err, core2.ErrContextLimitExceeded) {
				// One-shot guard: present only once per Run
				if ctx.Value(ctxKeyLimitRecoveryAttempted) == nil {
					ctx = context.WithValue(ctx, ctxKeyLimitRecoveryAttempted, true)
					if perr := s.presentContextLimitExceeded(ctx, genInput, strings.ReplaceAll(err.Error(), core2.ErrContextLimitExceeded.Error(), "")); perr != nil {
						return nil, fmt.Errorf("failed to handle context limit: %w", perr)
					}
				}
			}
			return nil, fmt.Errorf("failed to stream: %w", err)
		}
		wg.Wait()
		// propagate first tool error if any
		select {
		case toolErr := <-stepErrCh:
			if toolErr != nil {
				return nil, fmt.Errorf("tool execution failed: %w", toolErr)
			}
		default:
		}
		s.synthesizeFinalResponse(genOutput)
		//TODO if stream has not emit tool call but JSON (either eliciation or plan extract and act)

	} else {
		if err := s.llm.Generate(ctx, genInput, genOutput); err != nil {
			if errors.Is(err, core2.ErrContextLimitExceeded) {
				// One-shot guard: present only once per Run
				if ctx.Value(ctxKeyLimitRecoveryAttempted) == nil {
					ctx = context.WithValue(ctx, ctxKeyLimitRecoveryAttempted, true)
					if perr := s.presentContextLimitExceeded(ctx, genInput, strings.ReplaceAll(err.Error(), core2.ErrContextLimitExceeded.Error(), "")); perr != nil {
						return nil, fmt.Errorf("failed to handle context limit: %w", perr)
					}
				}
			}
			return nil, fmt.Errorf("failed to generate: %w", err)
		}
	}

	if aPlan.IsEmpty() {
		ok, err := s.extendPlanFromResponse(ctx, genOutput, aPlan)
		if ok {
			if err = s.streamPlanSteps(ctx, streamId, aPlan); err != nil {
				return nil, fmt.Errorf("failed to stream plan steps: %w", err)
			}
			wg.Wait()
			// propagate first tool error if any
			select {
			case toolErr := <-stepErrCh:
				if toolErr != nil {
					return nil, fmt.Errorf("tool execution failed: %w", toolErr)
				}
			default:
			}
		}
	}

	RefinePlan(aPlan)
	// If this turn executed message:remove, perform one retry generation automatically
	if hasRemovalTool(aPlan) {
		// Retry once to produce final assistant content with reduced context
		if err := s.llm.Generate(ctx, genInput, genOutput); err != nil {
			return nil, fmt.Errorf("retry after removal failed: %w", err)
		}
		// Extend/stream any additional steps if present
		if ok, _ := s.extendPlanFromResponse(ctx, genOutput, aPlan); ok {
			if err2 := s.streamPlanSteps(ctx, streamId, aPlan); err2 != nil {
				return nil, fmt.Errorf("failed to stream plan steps (retry): %w", err2)
			}
		}
	}
	return aPlan, nil
}

// hasRemovalTool returns true when the plan contains a message removal tool call.
func hasRemovalTool(p *plan.Plan) bool {
	if p == nil || len(p.Steps) == 0 {
		return false
	}
	for _, st := range p.Steps {
		name := strings.ToLower(strings.TrimSpace(st.Name))
		if name == "internal/message:remove" || name == "message:remove" || strings.HasSuffix(name, ":remove") {
			return true
		}
	}
	return false
}

// presentContextLimitExceeded composes a concise guidance note with removable-candidate lines,
// then triggers a best‑effort, tool‑driven recovery loop to free tokens (via internal/message tools),
// and finally inserts an assistant message with the guidance for the user.
func (s *Service) presentContextLimitExceeded(ctx context.Context, oldGenInput *core2.GenerateInput, errMessage string) error {
	convID := memory.ConversationIDFromContext(ctx)
	if strings.TrimSpace(convID) == "" || s.convClient == nil {
		return fmt.Errorf("missing conversation context")
	}
	// Fetch conversation with tool calls to build candidates
	conv, err := s.convClient.GetConversation(ctx, convID, apiconv.WithIncludeToolCall(true))
	if err != nil || conv == nil {
		return fmt.Errorf("failed to get conversation: %w", err)
	}
	lines, ids := s.buildRemovalCandidates(ctx, conv)
	if len(lines) == 0 {
		lines = []string{"(no removable items identified)"}
	}
	promptText := s.composeFreeTokenPrompt(errMessage, lines, ids)

	overlimit := 0
	if v, ok := extractOverlimitTokens(errMessage); ok {
		overlimit = v
		fmt.Printf("[debug] overlimit tokens: %d\n", overlimit)
	}

	err = s.freeMessageTokensLLM(ctx, conv, promptText, oldGenInput, overlimit)
	if err != nil {
		return fmt.Errorf("failed to free message tokens via LLM: %v\n", err)
	}

	// Insert assistant message in current conversation turn
	turn := s.ensureTurnMeta(ctx, conv)
	if _, aerr := apiconv.AddMessage(ctx, s.convClient, &turn,
		apiconv.WithRole("assistant"),
		apiconv.WithType("text"),
		apiconv.WithStatus("error"),
		apiconv.WithContent(promptText),
		apiconv.WithInterim(1),
	); aerr != nil {
		return fmt.Errorf("failed to insert context-limit message: %w", aerr)
	}

	return nil
}

// buildRemovalCandidates constructs concise one-line entries for removable items excluding the last user message.
func (s *Service) buildRemovalCandidates(ctx context.Context, conv *apiconv.Conversation) ([]string, []string) {
	if conv == nil {
		return nil, nil
	}
	tr := conv.GetTranscript()
	lastUserID := ""
	// Identify the last non-interim user message id
	for i := len(tr) - 1; i >= 0 && lastUserID == ""; i-- {
		t := tr[i]
		if t == nil || len(t.Message) == 0 {
			continue
		}
		for j := len(t.Message) - 1; j >= 0; j-- {
			m := t.Message[j]
			if m == nil || m.Interim != 0 || m.Content == nil || strings.TrimSpace(*m.Content) == "" {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(m.Role), "user") {
				lastUserID = m.Id
				break
			}
		}
	}
	// Build candidates
	const previewLen = 1000
	out := []string{}
	msgIDs := []string{}
	for _, t := range tr {
		if t == nil || len(t.Message) == 0 {
			continue
		}
		for _, m := range t.Message {
			if m == nil || m.Id == lastUserID || m.Interim != 0 || (m.Archived != nil && *m.Archived == 1) {
				continue
			}
			typ := strings.ToLower(strings.TrimSpace(m.Type))
			role := strings.ToLower(strings.TrimSpace(m.Role))
			if typ != "text" && m.ToolCall == nil {
				continue
			}
			// Build preview and size
			var line string
			if m.ToolCall != nil {
				toolName := strings.TrimSpace(m.ToolCall.ToolName)
				// args preview
				var args map[string]interface{}
				if m.ToolCall.RequestPayload != nil && m.ToolCall.RequestPayload.InlineBody != nil {
					raw := strings.TrimSpace(*m.ToolCall.RequestPayload.InlineBody)
					if raw != "" {
						var parsed map[string]interface{}
						if json.Unmarshal([]byte(raw), &parsed) == nil {
							args = parsed
						}
					}
				}
				argStr, _ := json.Marshal(args)
				ap := shared.RuneTruncate(string(argStr), previewLen)
				body := ""
				if m.ToolCall.ResponsePayload != nil && m.ToolCall.ResponsePayload.InlineBody != nil {
					body = *m.ToolCall.ResponsePayload.InlineBody
				}
				sz := len(body)
				line = fmt.Sprintf("messageId: %s, type: tool, tool: %s, args_preview: \"%s\", size: %d bytes (~%d tokens)", m.Id, toolName, ap, sz, estimateTokens(body))
			} else if role == "user" || role == "assistant" {
				body := ""
				if m.Content != nil {
					body = *m.Content
				}
				pv := shared.RuneTruncate(body, previewLen)
				sz := len(body)
				line = fmt.Sprintf("messageId: %s, type: %s, preview: \"%s\", size: %d bytes (~%d tokens)", m.Id, role, pv, sz, estimateTokens(body))
			} else {
				continue
			}
			out = append(out, line)
			msgIDs = append(msgIDs, m.Id)
		}
	}
	return out, msgIDs
}

// ensureTurnMeta returns a TurnMeta for adding messages: uses existing context when present, otherwise derives from conversation.
func (s *Service) ensureTurnMeta(ctx context.Context, conv *apiconv.Conversation) memory.TurnMeta {
	if tm, ok := memory.TurnMetaFromContext(ctx); ok {
		return tm
	}
	turnID := ""
	if conv != nil && conv.LastTurnId != nil {
		turnID = *conv.LastTurnId
	}
	return memory.TurnMeta{ConversationID: conv.Id, TurnID: turnID, ParentMessageID: turnID}
}

func estimateTokens(s string) int {
	return estimateTokensInt(len(s))
}

func estimateTokensInt(stringLength int) int {
	if stringLength == 0 {
		return 0
	}
	if stringLength < 8 {
		return 1
	}
	return (stringLength + 3) / 4
}

func (s *Service) streamPlanSteps(ctx context.Context, streamId string, aPlan *plan.Plan) error {
	handler, cleanup, err := stream.PrepareStreamHandler(ctx, streamId)
	if err != nil {
		return err
	}
	defer cleanup()
	for _, step := range aPlan.Steps {
		if err = handler(ctx, &llm.StreamEvent{
			Response: &llm.GenerateResponse{
				Choices: []llm.Choice{{
					Message: llm.Message{Role: llm.RoleAssistant,
						ToolCalls: []llm.ToolCall{{
							ID:        step.ID,
							Name:      step.Name,
							Arguments: step.Args,
						}},
						Content: step.Reason},
					FinishReason: "tool",
				}},
			},
		}); err != nil {
			return fmt.Errorf("failed to emit stream event: %w", err)
		}
	}
	return nil
}

func (s *Service) canStream(ctx context.Context, genInput *core2.GenerateInput) (bool, error) {
	genInput.MatchModelIfNeeded(s.llm.ModelMatcher())
	model, err := s.llm.ModelFinder().Find(ctx, genInput.Model)
	if err != nil {
		return false, err
	}
	doStream := model.Implements(base.CanStream)
	return doStream, nil
}

func (s *Service) registerStreamPlannerHandler(ctx context.Context, reg tool.Registry, aPlan *plan.Plan, wg *sync.WaitGroup, nextStepIdx *int, genOutput *core2.GenerateOutput) (string, <-chan error) {
	var mux sync.Mutex
	stepErrCh := make(chan error, 1)
	// Per-turn de-duplication: execute once per (tool+args) key and fan-out
	// the same result to subsequent identical call_ids.
	type cachedExec struct {
		result string
		err    error
		done   chan struct{}
	}
	dedup := map[string]*cachedExec{}
	var stopped atomic.Bool
	id := stream.Register(func(ctx context.Context, event *llm.StreamEvent) error {
		if stopped.Load() {
			return nil
		}
		if event == nil || event.Response == nil || len(event.Response.Choices) == 0 {
			if event != nil {
				return event.Err
			}
			return nil
		}
		choice := event.Response.Choices[0]
		mux.Lock()
		defer mux.Unlock()
		if content := strings.TrimSpace(choice.Message.Content); content != "" {
			if genOutput.Content == "" {
				genOutput.Content = content
			} else {
				genOutput.Content += content
			}
		}

		s.extendPlanWithToolCalls(event.Response.ResponseID, &choice, aPlan)

		for *nextStepIdx < len(aPlan.Steps) {
			st := aPlan.Steps[*nextStepIdx]
			*nextStepIdx++
			if st.Type != "tool" {
				continue
			}
			// De-dup by tool name + normalized args. Execute once; duplicates
			// will synthesize tool calls after the first completes.
			key := toolDedupKey(st.Name, st.Args)
			if key != "" {
				if ce, found := dedup[key]; found {
					// Duplicate: wait for canonical to finish and then synthesize this call
					step := st
					wg.Add(1)
					go func() {
						defer wg.Done()
						<-ce.done
						// Use cached result even if err != nil; completion flow records error fields.
						_ = executil.SynthesizeToolStep(ctx, s.convClient, executil.StepInfo{ID: step.ID, Name: step.Name, Args: step.Args, ResponseID: step.ResponseID}, ce.result)
					}()
					continue
				}
				// First occurrence: create cache entry with done channel
				dedup[key] = &cachedExec{done: make(chan struct{})}
			}
			wg.Add(1)
			step := st
			go func() {
				defer wg.Done()
				stepInfo := executil.StepInfo{ID: step.ID, Name: step.Name, Args: step.Args, ResponseID: step.ResponseID}
				// Execute tool; even on error we let the LLM decide next steps.
				// Errors are persisted on the tool call and exposed via tool result payload.
				out, _, _ := executil.ExecuteToolStep(ctx, reg, stepInfo, s.convClient)
				if key != "" {
					if ce := dedup[key]; ce != nil {
						ce.result = out.Result
						close(ce.done)
					}
				}
			}()
		}
		return nil
	})
	return id, stepErrCh
}

// toolDedupKey builds a stable key from tool name and arguments.
// It canonicalizes map/array structures so logically equivalent args
// produce identical keys independent of map iteration order.
func toolDedupKey(name string, args map[string]interface{}) string {
	b := &bytes.Buffer{}
	b.WriteString(strings.TrimSpace(strings.ToLower(name)))
	b.WriteString(":")
	writeCanonical(args, b)
	return b.String()
}

// writeCanonical appends a deterministic serialization of v to w.
func writeCanonical(v interface{}, w *bytes.Buffer) {
	switch t := v.(type) {
	case nil:
		w.WriteString("null")
	case string:
		w.WriteString("\"")
		w.WriteString(t)
		w.WriteString("\"")
	case bool:
		if t {
			w.WriteString("true")
		} else {
			w.WriteString("false")
		}
	case float64:
		// JSON numbers decode to float64
		w.WriteString(fmt.Sprintf("%g", t))
	case int, int32, int64:
		w.WriteString(fmt.Sprintf("%v", t))
	case map[string]interface{}:
		// sort keys
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		w.WriteString("{")
		for i, k := range keys {
			if i > 0 {
				w.WriteString(",")
			}
			w.WriteString("\"")
			w.WriteString(k)
			w.WriteString("\":")
			writeCanonical(t[k], w)
		}
		w.WriteString("}")
	case []interface{}:
		w.WriteString("[")
		for i, it := range t {
			if i > 0 {
				w.WriteString(",")
			}
			writeCanonical(it, w)
		}
		w.WriteString("]")
	default:
		// Fallback via fmt for other primitive types
		w.WriteString(fmt.Sprintf("%v", t))
	}
}

func (s *Service) extendPlanFromResponse(ctx context.Context, genOutput *core2.GenerateOutput, aPlan *plan.Plan) (bool, error) {
	if genOutput.Response == nil || len(genOutput.Response.Choices) == 0 {
		return false, nil
	}
	for j := range genOutput.Response.Choices {
		choice := &genOutput.Response.Choices[j]
		s.extendPlanWithToolCalls(genOutput.Response.ResponseID, choice, aPlan)
	}
	if len(aPlan.Steps) == 0 {
		if err := s.extendPlanFromContent(ctx, genOutput, aPlan); err != nil {
			return false, err
		}
	}
	return !aPlan.IsEmpty(), nil
}

func (s *Service) extendPlanWithToolCalls(responseID string, choice *llm.Choice, aPlan *plan.Plan) {
	if len(choice.Message.ToolCalls) == 0 {
		return
	}
	steps := make(plan.Steps, 0, len(choice.Message.ToolCalls))
	for _, tc := range choice.Message.ToolCalls {
		name := tc.Name
		args := tc.Arguments
		if name == "" && tc.Function.Name != "" {
			name = tc.Function.Name
		}
		if args == nil && tc.Function.Arguments != "" {
			var parsed map[string]interface{}
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &parsed); err == nil {
				args = parsed
			}
		}
		if prev := aPlan.Steps.Find(tc.ID); prev != nil {
			prev.Name = name
			prev.Args = args
			prev.Reason = choice.Message.Content
			continue
		}

		// for gemini compatibility
		if tc.ID == "" {
			tc.ID = "call_" + uuid.New().String()
		}

		steps = append(steps, plan.Step{
			ID:         tc.ID,
			Type:       "tool",
			Name:       name,
			Args:       args,
			Reason:     choice.Message.Content,
			ResponseID: strings.TrimSpace(responseID),
		})
	}
	aPlan.Steps = append(aPlan.Steps, steps...)
}

func (s *Service) extendPlanFromContent(ctx context.Context, genOutput *core2.GenerateOutput, aPlan *plan.Plan) error {
	var err error
	if strings.Contains(genOutput.Content, `"tool"`) {
		err = executil.EnsureJSONResponse(ctx, genOutput.Content, aPlan)
	}
	if strings.Contains(genOutput.Content, `"elicitation"`) {
		aPlan.Elicitation = &plan.Elicitation{}
		_ = executil.EnsureJSONResponse(ctx, genOutput.Content, aPlan.Elicitation)
		if aPlan.Elicitation.IsEmpty() {
			aPlan.Elicitation = nil
		} else {
			if aPlan.Elicitation.ElicitationId == "" {
				aPlan.Elicitation.ElicitationId = uuid.New().String()
			}
		}
	}

	aPlan.Steps.EnsureID()
	if len(aPlan.Steps) > 0 && strings.TrimSpace(aPlan.Steps[0].Reason) == "" {
		prefix := genOutput.Content
		if idx := strings.Index(prefix, "```json"); idx != -1 {
			prefix = prefix[:idx]
		} else if idx := strings.Index(prefix, "{"); idx != -1 {
			prefix = prefix[:idx]
		}
		prefix = strings.TrimSpace(prefix)
		if prefix != "" {
			aPlan.Steps[0].Reason = prefix
		}
	}
	return err
}

func (s *Service) synthesizeFinalResponse(genOutput *core2.GenerateOutput) {
	if strings.TrimSpace(genOutput.Content) == "" || genOutput.Response != nil {
		return
	}
	genOutput.Response = &llm.GenerateResponse{
		Choices: []llm.Choice{{
			Index:        0,
			Message:      llm.Message{Role: llm.RoleAssistant, Content: strings.TrimSpace(genOutput.Content)},
			FinishReason: "stop",
		}},
	}
}

func New(service *core2.Service, registry tool.Registry, convClient apiconv.Client, finder agentmdl.Finder, builder func(ctx context.Context, conv *apiconv.Conversation, instruction string) (*core2.GenerateInput, error)) *Service {
	return &Service{
		llm:            service,
		registry:       registry,
		convClient:     convClient,
		agentFinder:    finder,
		buildPlanInput: builder,
	}
}
