package agent

import (
	"context"
	_ "embed"
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

	// 2. Build conversation context string (excluding the current user message), then record the new user message.
	convID := s.conversationID(qi)
	convContext, err := s.conversationContext(ctx, convID, qi)
	if err != nil {
		return err
	}
	if err := s.addMessage(ctx, convID, "user", qi.Query); err != nil {
		log.Printf("warn: cannot record message: %v", err)
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
	sysPrompt, err := s.buildSystemPrompt(ctx, qi, enrichment)
	if err != nil {
		return err
	}
	wf, initial, err := s.loadWorkflow(qi, enrichment, sysPrompt)
	if err != nil {
		return err
	}
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
	if len(result.Errors) > 0 {
		// Instead of bubbling a hard error, surface a partial response so that
		// the user understands what went wrong and can decide how to proceed.
		errsJSON, _ := json.Marshal(result.Errors)
		var resSummary string
		if resRaw, ok := result.Output[keyResults]; ok {
			if b, err := json.Marshal(resRaw); err == nil {
				resSummary = string(b)
			}
		}

		qo.Content = fmt.Sprintf("I encountered an internal issue while composing the answer (details: %s).\n\nHere are the raw tool results I managed to obtain:\n%s", string(errsJSON), resSummary)
		qo.DocumentsSize = s.calculateDocumentsSize(docs)
		s.recordAssistant(ctx, convID, qo.Content)
		qo.Usage = usage.FromContext(ctx)
		return nil
	}

	// 4. Extract output – same logic but isolated.
	if elVal, ok := result.Output[keyElicitation]; ok && elVal != nil {
		if elic, err := coerceElicitation(elVal); err == nil && elic != nil {
			qo.Elicitation = elic
			qo.Content = elic.Message
			qo.DocumentsSize = s.calculateDocumentsSize(docs)
			s.recordAssistant(ctx, convID, qo.Content)
			qo.Usage = usage.FromContext(ctx)
			return nil
		}
	}

	ansRaw, ok := result.Output[keyAnswer]
	if !ok {
		// No explicit answer – check if workflow surfaced a tool error or at least
		// an error field inside individual results and propagate it so that the
		// user sees *something* instead of “[no response]”.

		// 1) Dedicated toolError field (added by exec service on fatal errors)
		if terr, ok2 := result.Output[keyToolError]; ok2 {
			qo.Content = fmt.Sprintf("tool error: %v", terr)
		} else {
			// 2) Scan step results for the first non-empty error string
			if resVal, ok3 := result.Output[keyResults]; ok3 {
				if items, ok4 := resVal.([]interface{}); ok4 {
					for _, it := range items {
						if m, ok5 := it.(map[string]interface{}); ok5 {
							if errStr, ok6 := m["error"].(string); ok6 && strings.TrimSpace(errStr) != "" {
								qo.Content = fmt.Sprintf("error: %s", errStr)
								break
							}
						}
					}
				}
			}
		}

		qo.Usage = usage.FromContext(ctx)
		qo.DocumentsSize = s.calculateDocumentsSize(docs)
		return nil
	}
	ansStr, ok := ansRaw.(string)
	if !ok {
		return fmt.Errorf("answer field of unexpected type %T", ansRaw)
	}

	qo.Content = ansStr
	qo.DocumentsSize = s.calculateDocumentsSize(docs)
	s.recordAssistant(ctx, convID, qo.Content)

	qo.Usage = usage.FromContext(ctx)

	// Fallback: if LLM returned an empty answer, try to surface the first
	// step-level error so that the user sees *something* explanatory.
	if strings.TrimSpace(qo.Content) == "" {
		if errMsg := firstStepError(result.Output); errMsg != "" {
			qo.Content = errMsg
		}
	}

	return nil
}

// firstStepError scans workflow output map for results and returns the first
// non-empty error string if found.
func firstStepError(out map[string]interface{}) string {
	resRaw, ok := out["results"]
	if !ok {
		return ""
	}
	items, ok := resRaw.([]interface{})
	if !ok {
		return ""
	}
	for _, it := range items {
		if m, ok2 := it.(map[string]interface{}); ok2 {
			if e, ok3 := m["error"].(string); ok3 && strings.TrimSpace(e) != "" {
				return e
			}
		}
	}
	return ""
}

// buildSystemPrompt constructs the system prompt for both workflows and direct answers.
func (s *Service) buildSystemPrompt(ctx context.Context, qi *QueryInput, enrichment string) (string, error) {
	sysPrompt, err := qi.Agent.GeneratePrompt(qi.Query, enrichment)
	if err != nil {
		return "", fmt.Errorf("prompt generation failed: %w", err)
	}
	return sysPrompt, nil
}

// recordAssistant writes the assistant's message into history, ignoring errors.
func (s *Service) recordAssistant(ctx context.Context, convID, content string) {
	if err := s.addMessage(ctx, convID, "assistant", content); err != nil {
		log.Printf("warn: cannot record assistant message: %v", err)
	}
}

// buildEnrichment merges conversation context and knowledge enrichment.
func (s *Service) buildEnrichment(conv, docs string) string {
	parts := []string{}
	if conv != "" {
		parts = append(parts, "Conversation:\n"+conv)
	}
	if docs != "" {
		parts = append(parts, "Documents:\n"+docs)
	}
	return strings.Join(parts, "\n\n")
}

//go:embed orchestration/workflow.yaml
var orchestrationWorkflow []byte

func (s *Service) loadWorkflow(qi *QueryInput, enrichment, systemPrompt string) (*model.Workflow, map[string]interface{}, error) {
	toolNames, err := s.ensureTools(qi)
	if err != nil {
		return nil, nil, err
	}

	var wf *model.Workflow
	if flow := qi.Agent.OrchestrationFlow; strings.TrimSpace(flow) != "" {
		wf, err = s.runtime.LoadWorkflow(context.Background(), flow)
	} else {
		wf, err = s.runtime.DecodeYAMLWorkflow(orchestrationWorkflow)
	}
	initial := map[string]interface{}{
		keyQuery:        qi.Query,
		keyContext:      enrichment,
		keyModel:        qi.Agent.Model,
		keyTools:        toolNames,
		keySystemPrompt: systemPrompt,
	}

	return wf, initial, err
}

func (s *Service) ensureTools(qi *QueryInput) ([]string, error) {
	var toolPatterns []string
	for _, aTool := range qi.Agent.Tool {
		pattern := aTool.Pattern
		if pattern == "" {
			pattern = aTool.Ref
		}
		if pattern == "" {
			pattern = aTool.Definition.Name
		}
		if pattern == "" {
			continue
		}
		toolPatterns = append(toolPatterns, pattern)
	}
	tools, err := s.registry.MustHaveTools(toolPatterns)
	if err != nil {
		return nil, err
	}
	var toolNames []string
	for _, aTool := range tools {
		if aTool.Definition.Name == "" {
			continue
		}
		toolNames = append(toolNames, aTool.Definition.Name)
	}
	return toolNames, err
}

// directAnswer produces an answer without tools / knowledge.
func (s *Service) directAnswer(ctx context.Context, qi *QueryInput, qo *QueryOutput, convID, convCtx string) error {
	sysPrompt, err := s.buildSystemPrompt(ctx, qi, convCtx)
	if err != nil {
		return err
	}

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
	s.recordAssistant(ctx, convID, qo.Content)

	qo.Usage = usage.FromContext(ctx)
	return nil
}

// coerceElicitation converts various representations found in workflow output
// into *plan.Elicitation.
func coerceElicitation(v interface{}) (*plan.Elicitation, error) {
	unmarshal := func(data []byte) (*plan.Elicitation, error) {
		var e plan.Elicitation
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		return &e, nil
	}
	switch actual := v.(type) {
	case *plan.Elicitation:
		return actual, nil
	case string:
		return unmarshal([]byte(actual))
	case []byte:
		return unmarshal(actual)
	case map[string]interface{}:
		data, _ := json.Marshal(actual)
		return unmarshal(data)
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
