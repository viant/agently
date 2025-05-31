package exec

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	plan2 "github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/extension/fluxor/llm/core"
	"regexp"
	"strconv"
	"strings"
)

//go:embed prompt/refine_plan.vm
var refinePromptTemplate string

// RunPlanInput defines input for executing a plan of steps.
type RunPlanInput struct {
	Plan           plan2.Plan `json:"plan,omitempty"`
	Model          string     `json:"model,omitempty"`
	Prompt         string     `json:"prompt,omitempty"` // optional prompt for final step
	Tools          []string   `json:"tools,omitempty"`
	Context        string     `json:"context,omitempty"`
	PromptTemplate string     `json:"promptTemplate,omitempty"` // optional custom prompt for Plan generation
}

// RunPlanOutput defines output for executing a plan of steps.
type RunPlanOutput struct {
	Results     []plan2.Result     `json:"results"`
	Elicitation *plan2.Elicitation `json:"elicitation,omitempty"`
}

// executePlan iterates over plan steps, calling tools for 'tool' steps.
func (s *Service) runPlan(ctx context.Context, in, out interface{}) error {
	input := in.(*RunPlanInput)
	output := out.(*RunPlanOutput)
	return s.RunPlan(ctx, input, output)
}

func (s *Service) RunPlan(ctx context.Context, input *RunPlanInput, output *RunPlanOutput) error {
	// Prepare structured results for each plan step
	var results []plan2.Result
	maxSteps := min(1000, s.maxSteps)
	planSteps := input.Plan.Steps
	totalSteps := 0

	// If planner indicated elicitation at plan level, propagate immediately.
	if elicitation := input.Plan.Elicitation; elicitation != nil && !elicitation.IsEmpty() {
		output.Elicitation = input.Plan.Elicitation
		return nil
	}

	promptTemplate := input.PromptTemplate
	if promptTemplate == "" {
		promptTemplate = refinePromptTemplate
	}
	tools, err := s.registry.MustHaveTools(input.Tools)
	if err != nil {
		return fmt.Errorf("failed to match tools: %w", err)
	}

	var toolError string
	var errorCount int

outer:
	for i := 0; i < len(planSteps); i++ {
		if maxSteps > 0 && totalSteps >= maxSteps {
			break
		}
		totalSteps++
		step := planSteps[i]
		switch step.Type {
		case "tool":
			// Resolve any $step[N].output placeholders before execution
			step.Args = resolveArgsPlaceholders(step.Args, results)
			callToolInput := &CallToolInput{
				Name:  step.Name,
				Args:  step.Args,
				Model: input.Model,
			}
			callToolOutput := &CallToolOutput{}
			if err := s.CallTool(ctx, callToolInput, callToolOutput); err != nil {
				toolError = fmt.Sprintf("failed to call tool %s: %v", step.Name, err)
				errorCount++
				break outer
			}
			results = append(results, plan2.Result{Name: step.Name, Args: step.Args, Result: callToolOutput.Result})

		case "clarify_intent":
			output.Results = results
			if len(step.MissingFields) > 0 {
				output.Elicitation = &plan2.Elicitation{Prompt: step.FollowupPrompt, MissingFields: step.MissingFields}
			} else {
				output.Elicitation = &plan2.Elicitation{Prompt: step.FollowupPrompt}
			}
			break outer
		case "abort":
			return errors.New(step.Reason)

		}
	}

	if errorCount > 10 {
		return fmt.Errorf("too many errors (%d) during plan execution, aborting", errorCount)
	}
	if len(results) > 0 {
		genInput := &core.GenerateInput{
			Template: promptTemplate,
			Model:    input.Model,
			Prompt:   input.Prompt,
			Bind: map[string]interface{}{

				"ExistingPlan": planSteps,
				"Results":      results, //TODO sumarize if needed it it's too large
				"Prompt":       input.Prompt,
				"Find":         input.Model,
				"Query":        input.Prompt,
				"Tools":        input.Tools,
				"Context":      input.Context,
				"ToolError":    toolError,
			},
			Tools: tools,
		}
		genOutput := &core.GenerateOutput{}
		if err := s.llm.Generate(ctx, genInput, genOutput); err != nil {
			return err
		}
		var refinedPlan plan2.Plan
		if err := core.EnsureJSONResponse(ctx, genOutput.Content, &refinedPlan); err == nil {
			if refinedPlan.IsRefined() {
				planSteps = refinedPlan.Steps
				goto outer
			}
		}
	}
	output.Results = results
	return nil
}

// ---------------------------------------------
// Placeholder resolution helpers
// ---------------------------------------------

var placeholderRegex = regexp.MustCompile(`^\$step\[(\d+)\]\.output(?:\.(.+))?$`)

// resolveArgsPlaceholders walks through the args map and substitutes any value
// of the form $step[N].output.<field>  or  $step[N].output with the referenced
// result from prior steps.
func resolveArgsPlaceholders(args map[string]interface{}, prior []plan2.Result) map[string]interface{} {
	if len(args) == 0 {
		return args
	}

	resolved := make(map[string]interface{}, len(args))
	for k, v := range args {
		switch tv := v.(type) {
		case string:
			if repl, ok := resolvePlaceholder(tv, prior); ok {
				resolved[k] = repl
			} else {
				resolved[k] = v
			}
		case map[string]interface{}:
			resolved[k] = resolveArgsPlaceholders(tv, prior)
		default:
			resolved[k] = v
		}
	}
	return resolved
}

// resolvePlaceholder attempts to resolve a single placeholder against prior
// results. It returns the resolved value and a boolean indicating success.
func resolvePlaceholder(raw string, prior []plan2.Result) (interface{}, bool) {
	m := placeholderRegex.FindStringSubmatch(strings.TrimSpace(raw))
	if len(m) == 0 {
		return nil, false
	}
	idxStr, fieldPath := m[1], m[2]
	idx, _ := strconv.Atoi(idxStr)
	if idx < 0 || idx >= len(prior) {
		return nil, false
	}
	// If no field path requested, return entire result string.
	base := prior[idx].Result
	if fieldPath == "" {
		return base, true
	}
	// Attempt JSON parse of result to extract field
	var doc interface{}
	if err := json.Unmarshal([]byte(base), &doc); err != nil {
		return nil, false
	}
	parts := strings.Split(fieldPath, ".")
	curr := doc
	for _, p := range parts {
		mm, ok := curr.(map[string]interface{})
		if !ok {
			return nil, false
		}
		curr, ok = mm[p]
		if !ok {
			return nil, false
		}
	}
	return curr, true
}
