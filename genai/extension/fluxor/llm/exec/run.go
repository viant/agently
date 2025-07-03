package exec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	plan "github.com/viant/agently/genai/agent/plan"
	core "github.com/viant/agently/genai/extension/fluxor/llm/core"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/tool"
	"github.com/viant/mcp-protocol/schema"
	"regexp"
	"strconv"
	"strings"
	"time"
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
	err := s.RunPlan(ctx, input, output)
	return err
}

func (s *Service) RunPlan(ctx context.Context, input *RunPlanInput, output *RunPlanOutput) error {
	// Prepare structured results for each plan step

	var results []plan.Result
	maxSteps := min(1000, s.maxSteps)
	planSteps := input.Plan.Steps

	totalSteps := 0
	// ------------------------------------------------------------------
	var stepTraceIDs []int
	conversationID := memory.ConversationIDFromContext(ctx)
	messageID := memory.MessageIDFromContext(ctx)
	if s.traceStore != nil && conversationID != "" {
		stepTraceIDs = make([]int, len(planSteps))

		for i, st := range planSteps {
			req, _ := json.Marshal(st.Args)

			skel := &memory.ExecutionTrace{
				Name:        st.Name,
				Request:     req,
				ParentMsgID: messageID,
				Success:     false,
				PlanID:      input.Plan.ID,
				StepIndex:   i,
				Step:        &planSteps[i],
			}
			tid, _ := s.traceStore.Add(ctx, conversationID, skel)
			stepTraceIDs[i] = tid
		}
	}

	// If planner indicated non-empty elicitation at plan level, propagate immediately.
	if e := input.Plan.Elicitation; e != nil && len(input.Plan.Steps) == 0 && !e.IsEmpty() {
		output.Elicitation = e
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
			startAt := time.Now()

			// use pre-populated trace id
			traceID := 0
			if len(stepTraceIDs) > i {
				traceID = stepTraceIDs[i]
			}
			if s.traceStore != nil && conversationID != "" && traceID > 0 {
				_ = s.traceStore.Update(ctx, conversationID, traceID, func(et *memory.ExecutionTrace) {
					et.StartedAt = startAt
				})
			}

			err := s.CallTool(ctx, callToolInput, callToolOutput)
			endAt := time.Now()

			fmt.Printf("RUN TOOL: req:%+v, resp: %+v err: %v\n", callToolInput, callToolOutput, err)
			result := plan.Result{Name: step.Name, Args: step.Args, Result: callToolOutput.Result, ID: step.ID}
			if err != nil {
				result.Error = err.Error()
				// missing param -> elicitation handled below
			}

			// ------------------------------------------------------------------
			// Record execution trace (best effort – ignore errors)
			// ------------------------------------------------------------------
			if s.traceStore != nil && conversationID != "" && traceID > 0 {
				var resp []byte

				if strings.HasPrefix(result.Result, "{") || strings.HasPrefix(result.Result, "[") && json.Valid([]byte(result.Result)) {
					resp = []byte(result.Result)
				} else {
					if data, err := json.Marshal(result.Result); err == nil {
						resp = data
					}
				}

				_ = s.traceStore.Update(ctx, conversationID, traceID, func(et *memory.ExecutionTrace) {
					et.Success = err == nil
					et.Result = resp
					et.Error = result.Error
					et.EndedAt = endAt
				})
			}

			// If missing params error recorded via err, build elicitation
			if err != nil {
				if def, ok := s.registry.GetDefinition(step.Name); ok {
					_, problems := tool.ValidateArgs(def, step.Args)
					if len(problems) > 0 {
						reqSchema := buildSchemaFromProblems(problems)
						output.Elicitation = &plan.Elicitation{
							ElicitRequestParams: schema.ElicitRequestParams{
								Message:         fmt.Sprintf("Tool %q requires additional parameters.", step.Name),
								RequestedSchema: reqSchema,
							},
						}
						break outer
					}
				}
			}
			results = append(results, result)

		case "clarify_intent": // backwards compatibility – treat same as "elicitation"
			fallthrough
		case "elicitation":
			// Record current results then exit with elicitation.
			output.Results = append(output.Results, results...)
			if step.Elicitation != nil {
				// The step already carries the full elicitation payload – forward as-is.
				output.Elicitation = step.Elicitation
			} else if step.Type == "clarify_intent" {
				// Fallback: Legacy models may send question/missing fields in args
				output.Elicitation = &plan.Elicitation{
					ElicitRequestParams: schema.ElicitRequestParams{Message: step.Content},
				}
			}
			break outer
		case "abort":
			return errors.New(step.Reason)

		}
	}

	// Ensure all accumulated results are surfaced.
	output.Results = append(output.Results, results...)

	// Deduplicate so that for any (tool,args) pair only the last execution result
	// is kept. This guarantees the planner receives a clean history without
	// repeated entries, yet still sees the most recent outcome.
	output.Results = dedupKeepLast(output.Results)
	return nil
}

// dedupKeepLast returns a new slice with only the last occurrence of each
// (tool, canonical-args) pair, preserving the chronological order of those
// last occurrences.
func dedupKeepLast(in []plan.Result) []plan.Result {
	if len(in) <= 1 {
		return in
	}

	type key struct {
		Name string
		Args string
	}

	seen := make(map[key]struct{}, len(in))
	outRev := make([]plan.Result, 0, len(in))

	// Walk backwards so we keep the *last* occurrence.
	for i := len(in) - 1; i >= 0; i-- {
		r := in[i]
		k := key{r.Name, core.CanonicalArgs(r.Args)}
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		outRev = append(outRev, r)
	}

	// Reverse back to chronological order.
	for i, j := 0, len(outRev)-1; i < j; i, j = i+1, j-1 {
		outRev[i], outRev[j] = outRev[j], outRev[i]
	}
	return outRev
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

// -----------------------------------------------------------------------------
// Elicitation helpers
// -----------------------------------------------------------------------------

// buildSchemaFromProblems converts a set of validation problems returned by
// tool.ValidateArgs into the restricted JSON schema payload expected by the
// elicitation protocol.
func buildSchemaFromProblems(problems []tool.Problem) schema.ElicitRequestParamsRequestedSchema {
	props := make(map[string]interface{}, len(problems))
	required := make([]string, 0, len(problems))
	for _, p := range problems {
		props[p.Name] = map[string]interface{}{"type": "string"}
		required = append(required, p.Name)
	}
	return schema.ElicitRequestParamsRequestedSchema{
		Type:       "object",
		Properties: props,
		Required:   required,
	}
}
