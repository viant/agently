package agent

import (
	"context"
	"fmt"
	"strings"

	plan "github.com/viant/agently/genai/agent/plan"
	core "github.com/viant/agently/genai/extension/fluxor/llm/core"
	execsvc "github.com/viant/agently/genai/extension/fluxor/llm/exec"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/memory"
)

// ReasonAndAct is a thin helper that composes a plan and executes it.
// It uses the new binding (BuildBinding) to render user and system context,
// generates a plan via llm/core and then runs the plan via llm/exec.
func (s *Service) ReasonAndAct(ctx context.Context, input *QueryInput) (*QueryOutput, error) {
	if s == nil {
		return nil, fmt.Errorf("agent service is nil")
	}
	if input == nil || input.Agent == nil {
		return nil, fmt.Errorf("invalid input: agent is required")
	}

	// 1) Build bindings and render contextual prompts
	sysBinding, err := s.BuildBinding(ctx, input, true)
	if err != nil {
		return nil, err
	}
	usrBinding, err := s.BuildBinding(ctx, input, false)
	if err != nil {
		return nil, err
	}

	var (
		systemPrompt string
		prompt       string
	)
	if input.Agent.SystemPrompt != nil {
		if txt, e := input.Agent.SystemPrompt.Generate(ctx, sysBinding); e == nil {
			systemPrompt = strings.TrimSpace(txt)
		} else {
			return nil, e
		}
	}
	if input.Agent.Prompt != nil {
		if txt, e := input.Agent.Prompt.Generate(ctx, usrBinding); e == nil {
			prompt = strings.TrimSpace(txt)
		} else {
			return nil, e
		}
	}

	// 2) Resolve model and tools
	model := strings.TrimSpace(input.ModelOverride)
	if model == "" {
		model = strings.TrimSpace(input.Agent.Model)
	}

	tools := make([]string, 0)
	if len(input.ToolsAllowed) > 0 {
		tools = append(tools, input.ToolsAllowed...)
	} else if len(input.Agent.Tool) > 0 {
		for _, t := range input.Agent.Tool {
			if t == nil || strings.TrimSpace(t.Definition.Name) == "" {
				continue
			}
			tools = append(tools, strings.TrimSpace(t.Definition.Name))
		}
	}

	// 3) Loop: create plan and execute until final answer or no progress
	var (
		allResults    []llm.ToolCall
		allTranscript []memory.Message
		finalAnswer   string
		lastPlan      *plan.Plan
	)

	const maxIterations = 10 //TODO
	for iter := 0; iter < maxIterations; iter++ {
		prevRes := len(allResults)
		prevTr := len(allTranscript)

		// Create or refine plan
		pIn := &core.PlanInput{
			Query:         input.Query,
			Context:       prompt,
			SystemContext: systemPrompt,
			Model:         model,
			Tools:         tools,
			Results:       append([]llm.ToolCall{}, allResults...),
			Transcript:    append([]memory.Message{}, allTranscript...),
			// Enable step execution during streaming when supported
			Runner: "llm/exec:run_plan",
		}
		var pOut core.PlanOutput
		if err := s.llm.Plan(ctx, pIn, &pOut); err != nil {
			return nil, err
		}

		// Accumulate plan, results, transcript
		lastPlan = pOut.Plan
		if len(pOut.Results) > 0 {
			allResults = append(allResults, pOut.Results...)
		}
		if len(pOut.Transcript) > 0 {
			allTranscript = append(allTranscript, pOut.Transcript...)
		}

		// Stop if final answer provided by LLM
		if ans := strings.TrimSpace(pOut.Answer); ans != "" {
			finalAnswer = ans
			break
		}

		// Execute plan locally when needed (non-streaming or partial execution)
		if pOut.Plan != nil && len(pOut.Plan.Steps) > 0 {
			exec := execsvc.New(s.llm, s.registry, model, nil, nil)
			var runOut execsvc.RunPlanOutput
			runIn := &execsvc.RunPlanInput{
				Plan:       *pOut.Plan,
				Model:      model,
				Tools:      tools,
				Results:    append([]llm.ToolCall{}, allResults...),
				Transcript: append([]memory.Message{}, allTranscript...),
				Context:    prompt,
			}
			_ = exec.RunPlan(ctx, runIn, &runOut) // best effort; keep iterating
			if len(runOut.Results) > 0 {
				allResults = append(allResults, runOut.Results...)
			}
			if runOut.Elicitation != nil && !runOut.Elicitation.IsEmpty() {
				return &QueryOutput{Agent: input.Agent, Elicitation: runOut.Elicitation, Plan: lastPlan, Model: model}, nil
			}
		}

		// No progress – stop to avoid infinite loop
		if len(allResults) == prevRes && len(allTranscript) == prevTr {
			break
		}
	}

	// 4) Build response
	out := &QueryOutput{Agent: input.Agent, Model: model, Plan: lastPlan}
	out.Content = finalAnswer
	return out, nil
}

// CreatePlanFromGenerateInput creates a plan using exact prompts supplied in gi.Content and gi.SystemPrompt.
// It bypasses binding/template generation and feeds prompts directly into llm/core.GeneratePlan.
// Tools are taken from gi.Tools and model from gi.Model (or the core's default when empty).
func (s *Service) CreatePlanFromGenerateInput(ctx context.Context, gi *core.GenerateInput) (*plan.Plan, []llm.ToolCall, error) {
	if s == nil || s.llm == nil {
		return nil, nil, fmt.Errorf("agent core service is nil")
	}
	if gi == nil {
		return nil, nil, fmt.Errorf("generate input is nil")
	}
	model := strings.TrimSpace(gi.Model)
	// Collect tool names for PlanInput
	toolNames := []string{}
	var tools []llm.Tool
	if gi.Binding.Tools != nil {
		for _, def := range gi.Binding.Tools.Signatures {
			if def == nil || strings.TrimSpace(def.Name) == "" {
				continue
			}
			toolNames = append(toolNames, def.Name)
			tools = append(tools, llm.NewFunctionTool(*def))
		}
	}
	genOut := &core.GenerateOutput{}
	// Feed the exact prompts as templates (no variables) into GeneratePlan.
	p, results, err := s.llm.GeneratePlan(ctx, model, gi.Prompt.Text, gi.SystemPrompt.Text, &core.PlanInput{Tools: toolNames}, tools, genOut)
	if err != nil {
		return nil, nil, err
	}
	return p, results, nil
}
