package exec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	plan "github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/tool"
	"regexp"
	"strconv"
	"strings"
)

// RunPlanInput defines input for executing a plan of steps.
// RunPlanInput defines input for executing a plan of steps.
type RunPlanInput struct {
	Plan       plan.Plan        `json:"plan,omitempty"`
	Model      string           `json:"model,omitempty"`
	Tools      []string         `json:"tools,omitempty"`
	Results    []plan.Result    `json:"results,omitempty"`
	Transcript []memory.Message `json:"transcript,omitempty"` // transcript of the conversation with the LLM`

	Context string `json:"context,omitempty"`
}

// RunPlanOutput defines output for executing a plan of steps.
type RunPlanOutput struct {
	Results     []plan.Result     `json:"results"`
	Transcript  []memory.Message  `json:"transcript,omitempty"` // transcript of the conversation with the LLM`
	Elicitation *plan.Elicitation `json:"elicitation,omitempty"`
}

// executePlan iterates over plan steps, calling tools for 'tool' steps.
func (s *Service) runPlan(ctx context.Context, in, out interface{}) error {
	input := in.(*RunPlanInput)
	output := out.(*RunPlanOutput)
	output.Results = input.Results
	return s.RunPlan(ctx, input, output)
}

func (s *Service) RunPlan(ctx context.Context, input *RunPlanInput, output *RunPlanOutput) error {
	// Prepare structured results for each plan step

	var results []plan.Result
	maxSteps := min(1000, s.maxSteps)
	planSteps := input.Plan.Steps
	totalSteps := 0

	// If planner indicated non-empty elicitation at plan level, propagate immediately.
	if e := input.Plan.Elicitation; e != nil && len(input.Plan.Steps) == 0 && !e.IsEmpty() {
		output.Elicitation = e
		fmt.Printf("Elicitation: %v\n", e)
		return nil
	}

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
			err := s.CallTool(ctx, callToolInput, callToolOutput)
			result := plan.Result{Name: step.Name, Args: step.Args, Result: callToolOutput.Result, ID: uuid.New().String()}
			if err != nil {
				result.Error = err.Error()
				// missing param -> elicitation handled below
			}

			// If missing params error recorded via err, build elicitation
			if err != nil {
				if def, ok := s.registry.GetDefinition(step.Name); ok {
					_, problems := tool.ValidateArgs(def, step.Args)
					if len(problems) > 0 {
						missing := make([]plan.MissingField, len(problems))
						for i, p := range problems {
							missing[i] = plan.MissingField{Name: p.Name}
						}
						output.Elicitation = &plan.Elicitation{
							Prompt:        fmt.Sprintf("Tool %q requires additional parameters.", step.Name),
							MissingFields: missing,
						}
						break outer
					}
				}
			}
			results = append(results, result)

		case "clarify_intent":
			// Record current results then exit with elicitation.
			output.Results = append(output.Results, results...)
			if len(step.MissingFields) > 0 {
				output.Elicitation = &plan.Elicitation{Prompt: step.FollowupPrompt, MissingFields: step.MissingFields}
			} else {
				output.Elicitation = &plan.Elicitation{Prompt: step.FollowupPrompt}
			}
			break outer
		case "abort":
			return errors.New(step.Reason)

		}
	}

	// Ensure all accumulated results are surfaced.
	output.Results = append(output.Results, results...)
	return nil
}

// ---------------------------------------------
// Placeholder resolution helpers
// ---------------------------------------------

var placeholderRegex = regexp.MustCompile(`^\$step\[(\d+)\]\.output(?:\.(.+))?$`)

// resolveArgsPlaceholders walks through the args map and substitutes any value
// of the form $step[N].output.<field>  or  $step[N].output with the referenced
// result from prior steps.
func resolveArgsPlaceholders(args map[string]interface{}, prior []plan.Result) map[string]interface{} {
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
func resolvePlaceholder(raw string, prior []plan.Result) (interface{}, bool) {
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

// extractShellError inspects the JSON returned by system/executor and returns
// a short error string when any command reports non-zero status or stderr.
func extractShellError(raw string) string {
	var doc struct {
		Commands []struct {
			Stderr string `json:"stderr"`
			Status int    `json:"status"`
		} `json:"commands"`
		Stderr string `json:"stderr"`
		Status int    `json:"status"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		// Fallback: try to extract "stderr":"..." via regex when the JSON
		// is not strictly valid (common when shell embeds unescaped newlines)
		re := regexp.MustCompile(`"stderr"\s*:\s*"([^"]+)"`)
		if m := re.FindStringSubmatch(raw); len(m) == 2 {
			return strings.TrimSpace(m[1])
		}
		return ""
	}
	// Overall status/stderr
	if doc.Status != 0 || strings.TrimSpace(doc.Stderr) != "" {
		if strings.TrimSpace(doc.Stderr) != "" {
			return strings.TrimSpace(doc.Stderr)
		}
		return fmt.Sprintf("command exited with status %d", doc.Status)
	}
	// Check individual commands (important for pipelines)
	for _, c := range doc.Commands {
		if c.Status != 0 || strings.TrimSpace(c.Stderr) != "" {
			if strings.TrimSpace(c.Stderr) != "" {
				return strings.TrimSpace(c.Stderr)
			}
			return fmt.Sprintf("command exited with status %d", c.Status)
		}
	}
	return ""
}
