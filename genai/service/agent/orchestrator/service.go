package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	chstore "github.com/viant/agently/client/chat/store"
	"github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/llm/provider/base"
	"github.com/viant/agently/genai/memory"
	core2 "github.com/viant/agently/genai/service/core"
	"github.com/viant/agently/genai/service/core/stream"
	executil "github.com/viant/agently/genai/service/shared/executil"
	"github.com/viant/agently/genai/tool"
)

type Service struct {
	llm        *core2.Service
	registry   tool.Registry
	convClient chstore.Client
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
	return aPlan, nil
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
				_, _, err := executil.ExecuteToolStep(ctx, reg, stepInfo, s.convClient)
				if err != nil {
					// capture first error and stop reacting to further events
					select {
					case stepErrCh <- err:
						stopped.Store(true)
					default:
					}
				}
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

func New(service *core2.Service, registry tool.Registry, convClient chstore.Client) *Service {
	return &Service{
		llm:        service,
		registry:   registry,
		convClient: convClient,
	}
}
