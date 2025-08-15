package exec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/viant/fluxor/model/types"

	"regexp"
	"strconv"
	"strings"
	"time"

	plan "github.com/viant/agently/genai/agent/plan"
	core "github.com/viant/agently/genai/extension/fluxor/llm/core"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/tool"
	"github.com/viant/mcp-protocol/schema"
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
	// Results accumulated during this invocation
	var results []plan.Result

	// Guard against pathological duplicate (tool,args) calls across iterations
	guard := core.NewDuplicateGuard(input.Results)
	executedByID := buildExecutedByID(input.Results)

	maxSteps := min(1000, s.maxSteps)
	planSteps := input.Plan.Steps

	// Initialise trace skeletons for each step (best effort)
	conversationID := memory.ConversationIDFromContext(ctx)
	messageID := memory.MessageIDFromContext(ctx)
	stepTraceIDs := s.initTraceSkeletons(ctx, planSteps, conversationID, messageID, input.Plan.ID)

	// Fast exit when plan-level elicitation is requested and there are no steps
	if e := input.Plan.Elicitation; e != nil && len(planSteps) == 0 && !e.IsEmpty() {
		output.Elicitation = e
		return nil
	}

	totalSteps := 0
outer:
	for i := 0; i < len(planSteps); i++ {
		if maxSteps > 0 && totalSteps >= maxSteps {
			break
		}
		totalSteps++

		step := planSteps[i]
		switch step.Type {
		case "tool":
			traceID := 0
			if len(stepTraceIDs) > i {
				traceID = stepTraceIDs[i]
			}
			if s.processToolStep(ctx, step, traceID, guard, executedByID, &results, input, output, conversationID) {
				break outer
			}

		case "clarify_intent": // backwards compatibility – treat same as "elicitation"
			fallthrough
		case "elicitation":
			if s.processElicitationStep(step, output, &results) {
				break outer
			}
		case "abort":
			return s.processAbortStep(step)

		}
	}

	// Ensure all accumulated results are surfaced.
	output.Results = append(output.Results, results...)

	return nil
}

// ---------------------- helpers ----------------------

func buildExecutedByID(prior []plan.Result) map[string]plan.Result {
	out := map[string]plan.Result{}
	for _, r := range prior {
		if r.ID != "" {
			out[r.ID] = r
		}
	}
	return out
}

// initTraceSkeletons creates initial ExecutionTrace entries per step and
// returns their IDs. Returns nil when traceStore or conversation is not set.
func (s *Service) initTraceSkeletons(ctx context.Context, steps plan.Steps, convID, parentMsgID, planID string) []int {
	if s.traceStore == nil || convID == "" {
		return nil
	}
	out := make([]int, len(steps))
	for i := range steps {
		req, _ := json.Marshal(steps[i].Args)
		skel := &memory.ExecutionTrace{Name: steps[i].Name, Request: req, ParentMsgID: parentMsgID, Success: false, PlanID: planID, StepIndex: i, Step: &steps[i]}
		tid, _ := s.traceStore.Add(ctx, convID, skel)
		out[i] = tid
	}
	return out
}

func (s *Service) updateTraceStart(ctx context.Context, convID string, traceID int, startAt time.Time) {
	if s.traceStore == nil || convID == "" || traceID <= 0 {
		return
	}
	_ = s.traceStore.Update(ctx, convID, traceID, func(et *memory.ExecutionTrace) { et.StartedAt = startAt })
}

func (s *Service) updateTraceEnd(ctx context.Context, convID string, traceID int, res plan.Result, duplicated bool, endAt time.Time) {
	if s.traceStore == nil || convID == "" || traceID <= 0 {
		return
	}
	// Marshal result payload to JSON
	var resp []byte
	if strings.HasPrefix(res.Result, "{") || strings.HasPrefix(res.Result, "[") && json.Valid([]byte(res.Result)) {
		resp = []byte(res.Result)
	} else if data, err := json.Marshal(res.Result); err == nil {
		resp = data
	}
	resErr := res.Error
	if duplicated {
		resErr = "WARN: duplicated call (cached result)"
	}
	_ = s.traceStore.Update(ctx, convID, traceID, func(et *memory.ExecutionTrace) {
		et.Success = resErr == ""
		et.Result = resp
		et.Error = resErr
		et.EndedAt = endAt
	})
}

// updateTraceWithPrev writes a previously computed result into the trace when
// a step is skipped due to matching tool-call ID.
func (s *Service) updateTraceWithPrev(ctx context.Context, convID string, traceID int, prev plan.Result) {
	if s.traceStore == nil || convID == "" || traceID <= 0 {
		return
	}
	var resp []byte
	if strings.HasPrefix(prev.Result, "{") || strings.HasPrefix(prev.Result, "[") && json.Valid([]byte(prev.Result)) {
		resp = []byte(prev.Result)
	} else if data, err := json.Marshal(prev.Result); err == nil {
		resp = data
	}
	now := time.Now()
	_ = s.traceStore.Update(ctx, convID, traceID, func(et *memory.ExecutionTrace) {
		if et.StartedAt.IsZero() {
			et.StartedAt = now
		}
		et.Success = prev.Error == ""
		et.Result = resp
		et.Error = prev.Error
		et.EndedAt = now
	})
}

// processToolStep executes or short-circuits a single tool step and updates traces/results.
// Returns true when the caller should break the outer loop (elicitation requested).
func (s *Service) processToolStep(ctx context.Context, step plan.Step, traceID int, guard *core.DuplicateGuard, executedByID map[string]plan.Result, results *[]plan.Result, input *RunPlanInput, output *RunPlanOutput, conversationID string) bool {
	// Skip execution when we already have a result for this tool-call ID.
	if step.ID != "" {
		if prev, done := executedByID[step.ID]; done {
			s.updateTraceWithPrev(ctx, conversationID, traceID, prev)
			return false
		}
	}

	// Duplicate-call heuristic across iterations
	duplicatedCall := false
	duplicatedResult := plan.Result{}
	if block, prev := guard.ShouldBlock(step.Name, step.Args); block {
		duplicatedResult = plan.Result{ID: step.ID, Name: step.Name, Args: step.Args, Result: prev.Result, Error: prev.Error}
		duplicatedCall = true
	}

	// Resolve placeholders
	step.Args = resolveArgsPlaceholders(step.Args, *results)
	callToolInput := &CallToolInput{Name: step.Name, Args: step.Args, Model: input.Model}
	callToolOutput := &CallToolOutput{}

	startAt := time.Now()
	s.updateTraceStart(ctx, conversationID, traceID, startAt)

	ctx = types.EnsureExecutionContext(ctx)

	var err error
	var endAt time.Time
	var result plan.Result
	if duplicatedCall {
		endAt = time.Now()
		result = duplicatedResult
	} else {
		err = s.CallTool(ctx, callToolInput, callToolOutput)
		endAt = time.Now()
		result = plan.Result{Name: step.Name, Args: step.Args, Result: callToolOutput.Result, ID: step.ID}
	}
	if err != nil {
		result.Error = err.Error()
	}

	// Update trace with actual or duplicated result
	s.updateTraceEnd(ctx, conversationID, traceID, result, duplicatedCall, endAt)

	// Elicitation on missing params (validation)
	if err != nil {
		if def, ok := s.registry.GetDefinition(step.Name); ok {
			if _, problems := tool.ValidateArgs(def, step.Args); len(problems) > 0 {
				reqSchema := buildSchemaFromProblems(problems)
				output.Elicitation = &plan.Elicitation{ElicitRequestParams: schema.ElicitRequestParams{Message: fmt.Sprintf("Tool %q requires additional parameters.", step.Name), RequestedSchema: reqSchema}}
				return true
			}
		}
	}
	*results = append(*results, result)
	guard.RegisterResult(step.Name, step.Args, result)
	return false
}

// processElicitationStep appends current results and sets elicitation in output; returns true to break loop.
func (s *Service) processElicitationStep(step plan.Step, output *RunPlanOutput, results *[]plan.Result) bool {
	output.Results = append(output.Results, *results...)
	if step.Elicitation != nil {
		output.Elicitation = step.Elicitation
	} else if step.Type == "clarify_intent" {
		output.Elicitation = &plan.Elicitation{ElicitRequestParams: schema.ElicitRequestParams{Message: step.Content}}
	}
	return true
}

// processAbortStep returns an error for abort steps.
func (s *Service) processAbortStep(step plan.Step) error {
	return errors.New(step.Reason)
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
