package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/viant/agently/genai/agent/plan"
	corepkg "github.com/viant/agently/genai/extension/fluxor/llm/core"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/tool"
	"github.com/viant/agently/genai/usage"
	"github.com/viant/fluxor/model"
	"github.com/viant/fluxor/model/types"
)

// --------------- Public entry -------------------------------------------------

// query is a Fluxor-executable that accepts *QueryInput and returns *QueryOutput.
// It is kept thin – orchestration is delegated to small helper methods so that
// each piece is individually testable.
func (s *Service) query(ctx context.Context, in, out interface{}) error {
	qi, ok := in.(*QueryInput)
	if !ok {
		return types.NewInvalidInputError(in)
	}
	qo, ok := out.(*QueryOutput)
	if !ok {
		return types.NewInvalidOutputError(out)
	}

	// 0. start token usage aggregation
	ctx, agg := usage.WithAggregator(ctx)

	// 1. Ensure we have an agent instance on the input.
	if err := s.ensureAgent(ctx, qi, qo); err != nil {
		return err
	}

	// 2. Record user message and build conversation context string.
	convID := s.conversationID(qi)
	if err := s.addMessage(ctx, convID, "user", qi.Query); err != nil {
		log.Printf("warn: cannot record message: %v", err)
	}

	convContext, err := s.conversationContext(ctx, convID, qi)
	if err != nil {
		return err
	}

	// 3. Decide whether to run the full plan/exec/finish workflow.
	if s.requiresWorkflow(qi) {
		return s.runWorkflow(ctx, qi, qo, convID, convContext)
	}

	// 4. Fallback – direct generation when query is present; otherwise nothing to do.
	if strings.TrimSpace(qi.Query) == "" {
		qo.Usage = agg
		return nil
	}
	err = s.directAnswer(ctx, qi, qo, convID, convContext)
	qo.Usage = agg
	return err
}

// --------------- Small helpers ------------------------------------------------

// ensureAgent populates qi.Agent (using finder when needed) and echoes it on
// qo.Agent for caller convenience.
func (s *Service) ensureAgent(ctx context.Context, qi *QueryInput, qo *QueryOutput) error {
	if qi.Agent == nil && qi.Location != "" {
		a, err := s.agentFinder.Find(ctx, qi.Location)
		if err != nil {
			return fmt.Errorf("failed to load agent: %w", err)
		}
		qi.Agent = a
	}
	if qi.Agent == nil {
		return fmt.Errorf("agent is required")
	}
	qo.Agent = qi.Agent
	return nil
}

func (s *Service) conversationID(qi *QueryInput) string {
	if qi.ConversationID != "" {
		return qi.ConversationID
	}
	return qi.Location
}

func (s *Service) addMessage(ctx context.Context, convID, role, content string) error {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	return s.history.AddMessage(ctx, convID, memory.Message{Role: role, Content: content})
}

// conversationContext summarises the conversation according to configured
// policies and returns a plain-text string.
func (s *Service) conversationContext(ctx context.Context, convID string, qi *QueryInput) (string, error) {
	summarizer := summarize(s, qi)
	policy := memory.NewCombinedPolicy(
		memory.NewSummaryPolicy(s.summaryThreshold, summarizer),
		memory.NewLastNPolicy(s.lastN),
	)

	msgs, err := s.history.Retrieve(ctx, convID, policy)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve conversation: %w", err)
	}
	var b strings.Builder
	for _, m := range msgs {
		b.WriteString(m.Role)
		b.WriteString(": ")
		b.WriteString(m.Content)
		b.WriteString("\n")
	}
	return b.String(), nil
}

// requiresWorkflow returns true when the agent has tools or knowledge AND a
// query is present – same business rule as before.
func (s *Service) requiresWorkflow(qi *QueryInput) bool {
	return len(qi.Agent.Knowledge) > 0 || len(qi.Agent.Tool) > 0
}

// runWorkflow executes the plan-exec-finish orchestration branch.
func (s *Service) runWorkflow(ctx context.Context, qi *QueryInput, qo *QueryOutput, convID, convCtx string) error {
	// 1. Retrieve documents for enrichment.
	//    When the current user query is empty, fall back to previous
	//    conversation context so that vector search still has a hint.
	var searchInput = *qi // shallow copy
	if strings.TrimSpace(searchInput.Query) == "" {
		searchInput.Query = convCtx
	}

	docs, err := s.retrieveRelevantDocuments(ctx, &searchInput)
	if err != nil {
		return fmt.Errorf("failed to retrieve knowledge: %w", err)
	}
	qo.Documents = docs

	// If agent has no tools AND no documents were found, running the workflow
	// would only waste tokens – fall back to direct answer.
	if len(qi.Agent.Tool) == 0 && len(docs) == 0 {
		return s.directAnswer(ctx, qi, qo, convID, convCtx)
	}

	enrichment := s.buildEnrichment(convCtx, s.formatDocumentsForEnrichment(docs, qi.IncludeFile))

	// 2. System prompt from agent template.
	sysPrompt, err := qi.Agent.GeneratePrompt(qi.Query, enrichment)
	if err != nil {
		return fmt.Errorf("prompt generation failed: %w", err)
	}
	if strings.TrimSpace(sysPrompt) != "" {
		enrichment = sysPrompt + "\n\n" + enrichment
	}

	wf, initial := s.buildWorkflow(qi, enrichment, sysPrompt)

	// inject policy mode
	if p := tool.FromContext(ctx); p != nil {
		initial[keyToolPolicy] = p.Mode
	}

	// 3. Run workflow
	_, wait, err := s.runtime.StartProcess(ctx, wf, initial)
	if err != nil {
		return fmt.Errorf("workflow start error: %w", err)
	}
	result, err := wait(ctx, s.workflowTimeout)
	if err != nil {
		return fmt.Errorf("workflow execution error: %w", err)
	}

	// 4. Extract output – same logic but isolated.
	if elVal, ok := result.Output[keyElicitation]; ok && elVal != nil {
		if elic, err := coerceElicitation(elVal); err == nil && elic != nil {
			qo.Elicitation = elic
			qo.Content = elic.Prompt
			qo.DocumentsSize = s.calculateDocumentsSize(docs)
			if err := s.addMessage(ctx, convID, "assistant", qo.Content); err != nil {
				log.Printf("warn: cannot record assistant message: %v", err)
			}
				qo.Usage = usage.FromContext(ctx)
			return nil
		}
	}

	ansRaw, ok := result.Output[keyAnswer]
	if !ok {
		return fmt.Errorf("workflow missing 'answer'; available keys=%v", mapKeys(result.Output))
	}
	ansStr, ok := ansRaw.(string)
	if !ok {
		return fmt.Errorf("answer field of unexpected type %T", ansRaw)
	}

	qo.Content = ansStr
	qo.DocumentsSize = s.calculateDocumentsSize(docs)
if err := s.addMessage(ctx, convID, "assistant", qo.Content); err != nil {
		log.Printf("warn: cannot record assistant message: %v", err)
	}

qo.Usage = usage.FromContext(ctx)
	return nil
}

// buildEnrichment merges conversation context and knowledge enrichment.
func (s *Service) buildEnrichment(conv, docs string) string {
	switch {
	case conv != "" && docs != "":
		return "Conversation:\n" + conv + "\nDocuments:\n" + docs
	case conv != "":
		return "Conversation:\n" + conv
	default:
		return docs
	}
}

func (s *Service) buildWorkflow(qi *QueryInput, enrichment, systemPrompt string) (*model.Workflow, map[string]interface{}) {
	wf := model.NewWorkflow("stage")

	// plan task
	wf.NewTask("plan").WithAction("llm/core", "plan", map[string]interface{}{
		keyQuery:   keyQueryPlaceholder,
		keyContext: keyContextPlaceholder,
		keyModel:   keyModelPlaceholder,
		keyTools:   keyToolsPlaceholder,
	}).WithPost(keyPlan, keyPlanPlaceholder)

	// exec task
	exec := wf.NewTask("exec").WithAction("llm/exec", "run_plan", map[string]interface{}{
		keyPlan:  keyPlanPlaceholder,
		keyModel: keyModelPlaceholder,
		keyTools: keyToolsPlaceholder,
		keyQuery: keyQueryPlaceholder,
	})
	exec.WithPost(keyResults+"[]", keyResultsPlaceholder)
	exec.WithPost(keyElicitation, keyElicitationPlaceholder)

	// finish task
	wf.NewTask("finish").WithAction("llm/core", "finalize", map[string]interface{}{
		keyQuery:   keyQueryPlaceholder,
		keyContext: keyContextPlaceholder,
		keyResults: keyResultsPlaceholder,
		keyModel:   keyModelPlaceholder,
	}).WithPost(keyAnswer, keyAnswerPlaceholder)

	// tool names for LLM planning context
	defs := s.registry.Definitions()
	names := make([]string, 0, len(defs))
	for _, d := range defs {
		names = append(names, d.Name)
	}

	initial := map[string]interface{}{
		keyQuery:        qi.Query,
		keyContext:      enrichment,
		keyModel:        qi.Agent.Model,
		keyTools:        names,
		keySystemPrompt: systemPrompt,
	}

	return wf, initial
}

// directAnswer produces an answer without tools / knowledge.
func (s *Service) directAnswer(ctx context.Context, qi *QueryInput, qo *QueryOutput, convID, convCtx string) error {
	sysPrompt, _ := qi.Agent.GeneratePrompt(qi.Query, convCtx)

	exec, err := s.llm.Method("generate")
	if err != nil {
		return err
	}

	genIn := &corepkg.GenerateInput{
		Model:        qi.Agent.Model,
		SystemPrompt: sysPrompt,
		Prompt:       qi.Query,
		Options:      &llm.Options{Temperature: qi.Agent.Temperature},
	}
	genOut := &corepkg.GenerateOutput{}
	if err := exec(ctx, genIn, genOut); err != nil {
		return err
	}

	qo.Content = genOut.Content
if err := s.addMessage(ctx, convID, "assistant", qo.Content); err != nil {
		log.Printf("warn: cannot record assistant message: %v", err)
	}

	qo.Usage = usage.FromContext(ctx)
	return nil
}

// coerceElicitation converts various representations found in workflow output
// into *plan.Elicitation.
func coerceElicitation(v interface{}) (*plan.Elicitation, error) {
	switch actual := v.(type) {
	case *plan.Elicitation:
		return actual, nil
	case string:
		var e plan.Elicitation
		if err := json.Unmarshal([]byte(actual), &e); err != nil {
			return nil, err
		}
		return &e, nil
	case []byte:
		var e plan.Elicitation
		if err := json.Unmarshal(actual, &e); err != nil {
			return nil, err
		}
		return &e, nil
	case map[string]interface{}:
		data, _ := json.Marshal(actual)
		var e plan.Elicitation
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		return &e, nil
	default:
		return nil, fmt.Errorf("unsupported elicitation type %T", v)
	}
}

// mapKeys returns sorted keys of a map for log/error purposes.
func mapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
