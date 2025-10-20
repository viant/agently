package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/llm/provider/base"
	"github.com/viant/agently/genai/memory"
	core2 "github.com/viant/agently/genai/service/core"
	"github.com/viant/agently/genai/service/core/stream"
	executil "github.com/viant/agently/genai/service/shared/executil"
	"github.com/viant/agently/genai/tool"
	agconv "github.com/viant/agently/pkg/agently/conversation"
)

type Service struct {
	llm        *core2.Service
	registry   tool.Registry
	convClient apiconv.Client
}

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
		//TODO if strem has not emit tool call but JSON (either eliciation or plan extract and act)

	} else {
		if err := s.llm.Generate(ctx, genInput, genOutput); err != nil {
			if errors.Is(err, core2.ErrContextLimitExceeded) {
				// Present token-limit message with candidates to guide removal
				if perr := s.presentContextLimitExceeded(ctx); perr != nil {
					return nil, fmt.Errorf("failed to handle context limit: %w", perr)
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

// presentContextLimitExceeded inserts an assistant message explaining the context-limit error
// and listing concise candidates to remove. It uses a minimal auto-compaction pass only to
// free space for this presentation message. Subsequent cleanup is LLM-driven via message:remove
// (and optionally message:summarize).
func (s *Service) presentContextLimitExceeded(ctx context.Context) error {
	convID := memory.ConversationIDFromContext(ctx)
	if strings.TrimSpace(convID) == "" || s.convClient == nil {
		return fmt.Errorf("missing conversation context")
	}
	// Fetch conversation with tool calls to build candidates
	conv, err := s.convClient.GetConversation(ctx, convID, apiconv.WithIncludeToolCall(true))
	if err != nil || conv == nil {
		return fmt.Errorf("failed to get conversation: %w", err)
	}
	lines := s.buildRemovalCandidates(ctx, conv)
	if len(lines) == 0 {
		// Provide minimal instruction even without candidates
		lines = []string{"(no removable items identified)"}
	}
	// Compose presentation message
	var buf bytes.Buffer
	buf.WriteString("Context limit exceeded: reduce context to continue.\n")
	buf.WriteString("Use tools message:remove (to remove items) and message:summarize (to craft short summaries). We will retry automatically after removals.\n")
	buf.WriteString("Candidates:\n")
	for _, l := range lines {
		buf.WriteString(l)
		buf.WriteByte('\n')
	}

	// Make room for the presentation message by auto-compacting oldest items (excluding the last user message).
	// This compaction is limited in scope and is not used for general cleanup.
	needed := estimateTokens(buf.String()) + 128 // small safety margin
	if aerr := s.autoCompactToFitPresentation(ctx, conv, needed); aerr != nil {
		// Best-effort: continue even if compact fails
	}

	// Insert assistant message in current conversation turn
	turn := s.ensureTurnMeta(ctx, conv)
	if _, aerr := apiconv.AddMessage(ctx, s.convClient, &turn,
		apiconv.WithRole("assistant"),
		apiconv.WithType("text"),
		apiconv.WithStatus("error"),
		apiconv.WithContent(buf.String()),
	); aerr != nil {
		return fmt.Errorf("failed to insert context-limit message: %w", aerr)
	}
	return nil
}

// buildRemovalCandidates constructs concise one-line entries for removable items excluding the last user message.
func (s *Service) buildRemovalCandidates(ctx context.Context, conv *apiconv.Conversation) []string {
	if conv == nil {
		return nil
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
	const previewLen = 100
	out := []string{}
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
				ap := string(argStr)
				if len(ap) > previewLen {
					ap = ap[:previewLen]
				}
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
				pv := body
				if len(pv) > previewLen {
					pv = pv[:previewLen]
				}
				sz := len(body)
				line = fmt.Sprintf("messageId: %s, type: %s, preview: \"%s\", size: %d bytes (~%d tokens)", m.Id, role, pv, sz, estimateTokens(body))
			} else {
				continue
			}
			out = append(out, line)
		}
	}
	return out
}

// autoCompactToFitPresentation archives oldest messages to free at least neededTokens, excluding the last user message.
// It inserts a short assistant summary per removed message to retain context.
func (s *Service) autoCompactToFitPresentation(ctx context.Context, conv *apiconv.Conversation, neededTokens int) error {
	if neededTokens <= 0 || conv == nil || s.convClient == nil {
		return nil
	}
	tr := conv.GetTranscript()
	// Identify last user message to preserve
	lastUserID := ""
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
	// Helper to attempt removal for a message view
	tryRemove := func(m *agconv.MessageView) (freed int, ok bool) {
		if m == nil || m.Id == lastUserID || (m.Archived != nil && *m.Archived == 1) || m.Interim != 0 {
			return 0, false
		}
		typ := strings.ToLower(strings.TrimSpace(m.Type))
		role := strings.ToLower(strings.TrimSpace(m.Role))
		var body string
		if m.ToolCall != nil {
			if m.ToolCall.ResponsePayload != nil && m.ToolCall.ResponsePayload.InlineBody != nil {
				body = *m.ToolCall.ResponsePayload.InlineBody
			}
		} else if typ == "text" && (role == "user" || role == "assistant") {
			if m.Content != nil {
				body = *m.Content
			}
		} else {
			return 0, false
		}
		if strings.TrimSpace(body) == "" {
			return 0, false
		}
		freed = estimateTokens(body)
		// Insert a short assistant summary
		preview := body
		if len(preview) > 100 {
			preview = preview[:100]
		}
		sum := preview
		// Include tool hint for tool calls
		if m.ToolCall != nil {
			tool := strings.TrimSpace(m.ToolCall.ToolName)
			var args map[string]interface{}
			if m.ToolCall.RequestPayload != nil && m.ToolCall.RequestPayload.InlineBody != nil {
				raw := strings.TrimSpace(*m.ToolCall.RequestPayload.InlineBody)
				if raw != "" {
					var parsed map[string]interface{}
					_ = json.Unmarshal([]byte(raw), &parsed)
					args = parsed
				}
			}
			argStr, _ := json.Marshal(args)
			ap := string(argStr)
			if len(ap) > 100 {
				ap = ap[:100]
			}
			sum = fmt.Sprintf("%s: %s", tool, ap)
		}
		turn := s.ensureTurnMeta(ctx, conv)
		if _, err := apiconv.AddMessage(ctx, s.convClient, &turn,
			apiconv.WithRole("assistant"), apiconv.WithType("text"), apiconv.WithStatus("summary"), apiconv.WithContent(sum)); err != nil {
			return 0, false
		}
		// Archive the original
		mm := apiconv.NewMessage()
		mm.SetId(m.Id)
		mm.SetArchived(1)
		if err := s.convClient.PatchMessage(ctx, mm); err != nil {
			return 0, false
		}
		return freed, true
	}

	freed := 0
	// Pass 1: oldest user/assistant text
	for _, t := range tr {
		if t == nil {
			continue
		}
		for _, m := range t.Message {
			if m == nil {
				continue
			}
			role := strings.ToLower(strings.TrimSpace(m.Role))
			typ := strings.ToLower(strings.TrimSpace(m.Type))
			if typ == "text" && (role == "user" || role == "assistant") {
				if f, ok := tryRemove(m); ok {
					freed += f
					if freed >= neededTokens {
						return nil
					}
				}
			}
		}
	}
	// Pass 2: oldest tool-call results
	for _, t := range tr {
		if t == nil {
			continue
		}
		for _, m := range t.Message {
			if m == nil || m.ToolCall == nil {
				continue
			}
			if f, ok := tryRemove(m); ok {
				freed += f
				if freed >= neededTokens {
					return nil
				}
			}
		}
	}
	return nil
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
	if s == "" {
		return 0
	}
	n := len(s)
	if n < 8 {
		return 1
	}
	return (n + 3) / 4
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

		s.extendPlanWithToolCalls(&choice, aPlan)

		for *nextStepIdx < len(aPlan.Steps) {
			st := aPlan.Steps[*nextStepIdx]
			*nextStepIdx++
			if st.Type != "tool" {
				continue
			}
			wg.Add(1)
			step := st
			go func() {
				defer wg.Done()
				stepInfo := executil.StepInfo{ID: step.ID, Name: step.Name, Args: step.Args}
				// Execute tool; even on error we let the LLM decide next steps.
				// Errors are persisted on the tool call and exposed via tool result payload.
				_, _, _ = executil.ExecuteToolStep(ctx, reg, stepInfo, s.convClient)
			}()
		}
		return nil
	})
	return id, stepErrCh
}

func (s *Service) extendPlanFromResponse(ctx context.Context, genOutput *core2.GenerateOutput, aPlan *plan.Plan) (bool, error) {
	if genOutput.Response == nil || len(genOutput.Response.Choices) == 0 {
		return false, nil
	}
	for j := range genOutput.Response.Choices {
		choice := &genOutput.Response.Choices[j]
		s.extendPlanWithToolCalls(choice, aPlan)
	}
	if len(aPlan.Steps) == 0 {
		if err := s.extendPlanFromContent(ctx, genOutput, aPlan); err != nil {
			return false, err
		}
	}
	return !aPlan.IsEmpty(), nil
}

func (s *Service) extendPlanWithToolCalls(choice *llm.Choice, aPlan *plan.Plan) {
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
		steps = append(steps, plan.Step{
			ID:     tc.ID,
			Type:   "tool",
			Name:   name,
			Args:   args,
			Reason: choice.Message.Content,
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

func New(service *core2.Service, registry tool.Registry, convClient apiconv.Client) *Service {
	return &Service{
		llm:        service,
		registry:   registry,
		convClient: convClient,
	}
}
