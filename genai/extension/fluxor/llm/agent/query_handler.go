package agent

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/viant/afs/file"
	"github.com/viant/afs/url"
	agentmdl "github.com/viant/agently/genai/agent"
	"github.com/viant/agently/internal/workspace"
	"log"
	"path"
	"strings"

	"github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/elicitation/refiner"
	corepkg "github.com/viant/agently/genai/extension/fluxor/llm/core"
	autoawait "github.com/viant/agently/genai/io/elicitation/auto"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/summary"
	"github.com/viant/agently/genai/tool"
	"github.com/viant/agently/genai/usage"
	"github.com/viant/fluxor/model"
	"github.com/viant/fluxor/model/types"
	"time"
)

// validateContext checks whether all required properties defined by the
// elicitation schema are present inside the caller supplied Context map. It
// returns a slice with the names of missing properties (empty slice when the
// context satisfies the schema).
// validateContext checks whether all required properties defined by the
// agent-level elicitation schema are present in the caller supplied Context
// map. It returns a slice with the names of missing properties (empty slice
// when the context satisfies the schema or no schema is defined).
func validateContext(qi *QueryInput) []string {
	if qi == nil || qi.Agent == nil || qi.Agent.Elicitation == nil {
		return nil
	}
	rSchema := qi.Agent.Elicitation.RequestedSchema
	if len(rSchema.Required) == 0 {
		return nil // nothing explicitly required
	}
	missing := make([]string, 0)
	for _, prop := range rSchema.Required {
		if _, ok := qi.Context[prop]; !ok {
			missing = append(missing, prop)
		}
	}
	return missing
}

// tryAutoElicit attempts to satisfy the missing context properties using an
// automatic elicitation agent. The first implementation delegates to a stub
// awaiter that always declines so that future phases can replace it with real
// LLM logic without touching call sites.
func (s *Service) tryAutoElicit(ctx context.Context, qi *QueryInput, missing []string) (map[string]any, bool) {
	helper := "elicitor"
	if s.defaults != nil && strings.TrimSpace(s.defaults.Agent) != "" {
		helper = s.defaults.Agent
	}

	caller := func(ctx context.Context, agentName, prompt string) (string, error) {
		in := &QueryInput{AgentName: agentName, Query: prompt, Persona: &agentmdl.Persona{Role: "assistant", Actor: "auto-elicitation"}}
		var out QueryOutput
		if err := s.query(ctx, in, &out); err != nil {
			return "", err
		}
		return out.Content, nil
	}

	awaiter := autoawait.New(caller, autoawait.Config{HelperAgent: helper, MaxRounds: 1})
	res, err := awaiter.AwaitElicitation(ctx, qi.Agent.Elicitation)
	if err != nil || res == nil {
		return nil, false
	}
	if res.Action != plan.ElicitResultActionAccept || len(res.Payload) == 0 {
		return nil, false
	}
	for _, m := range missing {
		if _, ok := res.Payload[m]; !ok {
			return nil, false
		}
	}
	return res.Payload, true
}

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

	// ------------------------------------------------------------------
	// 0.a Persist / restore per-conversation model override.
	// ------------------------------------------------------------------
	earlyConvID := s.conversationID(qi)
	if hs, ok := s.history.(*memory.HistoryStore); ok && earlyConvID != "" {
		if qi.ModelOverride == "" {
			if meta, ok := hs.Meta(ctx, earlyConvID); ok && meta.Model != "" {
				qi.ModelOverride = meta.Model
			}
		} else {
			// Store/overwrite the chosen model so that subsequent turns inherit it.
			hs.UpdateMeta(ctx, earlyConvID, func(m *memory.ConversationMeta) { m.Model = qi.ModelOverride })
		}

		// ------------------------------------------------------------
		// Persist / restore per-conversation allowed tool list.
		// ------------------------------------------------------------
		if len(qi.ToolsAllowed) == 0 {
			if meta, ok := hs.Meta(ctx, earlyConvID); ok && len(meta.Tools) > 0 {
				// copy to avoid accidental mutation by caller
				qi.ToolsAllowed = append([]string(nil), meta.Tools...)
			}
		} else {
			allowedCopy := append([]string(nil), qi.ToolsAllowed...)
			hs.UpdateMeta(ctx, earlyConvID, func(m *memory.ConversationMeta) { m.Tools = allowedCopy })
		}

		// ------------------------------------------------------------
		// Persist / restore agent reference.
		// ------------------------------------------------------------
		chosenAgent := ""
		if strings.TrimSpace(qi.AgentName) != "" {
			chosenAgent = qi.AgentName
		} else if qi.Agent != nil && strings.TrimSpace(qi.Agent.Name) != "" {
			chosenAgent = qi.Agent.Name
		}

		if chosenAgent == "" {
			if meta, ok := hs.Meta(ctx, earlyConvID); ok && meta.Agent != "" {
				qi.AgentName = meta.Agent
			}
		} else {
			// store
			hs.UpdateMeta(ctx, earlyConvID, func(m *memory.ConversationMeta) { m.Agent = chosenAgent })
		}
	}

	// ------------------------------------------------------------
	// Treat current user query as potential JSON elicitation payload and
	// merge into context *before* validation so the same turn can succeed
	// without another round-trip.
	// ------------------------------------------------------------
	if strings.TrimSpace(qi.Query) != "" {
		var tmp map[string]interface{}
		if err := json.Unmarshal([]byte(strings.TrimSpace(qi.Query)), &tmp); err == nil && len(tmp) > 0 {
			if qi.Context == nil {
				qi.Context = map[string]interface{}{}
			}
			for k, v := range tmp {
				if _, exists := qi.Context[k]; !exists {
					qi.Context[k] = v
				}
			}
		}
	}

	// ------------------------------------------------------------------
	// 0.a Ensure agent is loaded (required for context validation below)
	if err := s.ensureAgent(ctx, qi, qo); err != nil {
		return err
	}

	// ------------------------------------------------------------------
	// 0.b Optional: validate context against agent's elicitation schema
	if qi.Agent.Elicitation != nil {
		// First attempt to auto-fill missing context properties from previous user
		// messages that contain JSON objects.
		s.enrichContextFromHistory(ctx, qi)

		if missing := validateContext(qi); len(missing) > 0 {
			// Attempt auto-elicitation when mode == agent|hybrid.
			autoAttempted := false
			if qi.ElicitationMode == "agent" || qi.ElicitationMode == "hybrid" {
				if payload, ok := s.tryAutoElicit(ctx, qi, missing); ok {
					if qi.Context == nil {
						qi.Context = map[string]any{}
					}
					for k, v := range payload {
						qi.Context[k] = v
					}
					// Re-run validation after auto-fill.
					if len(validateContext(qi)) == 0 {
						// Auto fill succeeded – proceed without raising elicitation.
						goto CONTINUE_PROCESSING
					}
				}
				autoAttempted = true
			}

			if qi.ElicitationMode == "agent" && autoAttempted {
				return fmt.Errorf("auto-elicitation failed to satisfy schema")
			}

			// Ensure the initiating user message is stored so that the UI shows it.
			convID := s.conversationID(qi)
			if _, err := s.addMessage(ctx, convID, "user", "", qi.Query, qi.MessageID, ""); err != nil {
				log.Printf("warn: cannot record initial user message: %v", err)
			}

			// Context is still incomplete – ask the caller for the remaining fields.
			qo.Elicitation = qi.Agent.Elicitation
			qo.Content = qi.Agent.Elicitation.Message
			qo.Usage = agg
			s.recordAssistantElicitation(ctx, convID, qi.MessageID, qo.Elicitation)

			// Store any provided context fragments to avoid repeated prompts.
			if hs, ok := s.history.(*memory.HistoryStore); ok && convID != "" {
				hs.UpdateMeta(ctx, convID, func(m *memory.ConversationMeta) {
					if m.Context == nil {
						m.Context = map[string]interface{}{}
					}
					for k, v := range qi.Context {
						m.Context[k] = v
					}
				})
			}
			return nil // early exit – wait for user input
		}

	CONTINUE_PROCESSING:
	}

	// 0.c Apply per-call tool policy if ToolsAllowed present
	if len(qi.ToolsAllowed) > 0 {
		pol := &tool.Policy{Mode: tool.ModeAuto, AllowList: qi.ToolsAllowed}
		ctx = tool.WithPolicy(ctx, pol)
	}

	// 1. Build conversation context string (excluding the current user message), then record the new user message.
	convID := s.conversationID(qi)
	convContext, err := s.conversationContext(ctx, convID, qi)
	if err != nil {
		return err
	}

	// Persist completed context so that future turns inherit all values.
	if hs, ok := s.history.(*memory.HistoryStore); ok && convID != "" && len(qi.Context) > 0 {
		ctxCopy := map[string]interface{}{}
		for k, v := range qi.Context {
			ctxCopy[k] = v
		}
		hs.UpdateMeta(ctx, convID, func(m *memory.ConversationMeta) { m.Context = ctxCopy })
	}
	messageID, err := s.addMessage(ctx, convID, "user", "", qi.Query, qi.MessageID, "")
	if err != nil {
		log.Printf("warn: cannot record message: %v", err)
	}
	ctx = context.WithValue(ctx, memory.MessageIDKey, messageID)
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

// enrichContextFromHistory scans existing user messages in the conversation
// for JSON objects and copies any fields that are required by the elicitation
// schema but not yet present in qi.Context.
func (s *Service) enrichContextFromHistory(ctx context.Context, qi *QueryInput) {
	if qi == nil || qi.Agent == nil || qi.Agent.Elicitation == nil {
		return
	}
	convID := s.conversationID(qi)
	if convID == "" || s.history == nil {
		return
	}

	// Build a lookup of missing keys.
	required := make(map[string]struct{})
	for _, k := range qi.Agent.Elicitation.RequestedSchema.Required {
		if qi.Context != nil {
			if _, ok := qi.Context[k]; ok {
				continue // already satisfied
			}
		}
		required[k] = struct{}{}
	}
	if len(required) == 0 {
		return
	}

	msgs, err := s.history.GetMessages(ctx, convID)
	if err != nil || len(msgs) == 0 {
		return
	}

	for _, m := range msgs {
		if len(required) == 0 {
			break // all satisfied
		}
		if m.Role != "user" {
			continue
		}
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(strings.TrimSpace(m.Content)), &obj); err != nil {
			continue // not a JSON object
		}
		if len(obj) == 0 {
			continue
		}
		if qi.Context == nil {
			qi.Context = map[string]interface{}{}
		}
		for k := range required {
			if val, ok := obj[k]; ok {
				qi.Context[k] = val
				delete(required, k)
			}
		}
	}
}

// --------------- Small helpers ------------------------------------------------

// ensureAgent populates qi.Agent (using finder when needed) and echoes it on
// qo.Agent for caller convenience.
func (s *Service) ensureAgent(ctx context.Context, qi *QueryInput, qo *QueryOutput) error {
	if qi.Agent == nil && qi.AgentName != "" {
		a, err := s.agentFinder.Find(ctx, qi.AgentName)
		if err != nil {
			return fmt.Errorf("failed to load agent: %w", err)
		}
		qi.Agent = a
	}
	if qi.Agent == nil {
		return fmt.Errorf("agent is required")
	}
	qo.Agent = qi.Agent

	// Apply model override when supplied
	if qi.ModelOverride != "" {
		qi.Agent.Model = qi.ModelOverride
	}
	return nil
}

func (s *Service) conversationID(qi *QueryInput) string {
	if qi.ConversationID != "" {
		return qi.ConversationID
	}
	return qi.AgentName
}

// addMessage appends a new message to the conversation history. When
// idOverride is non-empty it is used as the message ID; otherwise a fresh UUID
// is generated. The final ID is always returned so that callers can propagate
// it via context for downstream services.

func (s *Service) addMessage(ctx context.Context, convID, role, actor, content, id string, parentId string) (string, error) {
	if strings.TrimSpace(content) == "" {
		return "", nil
	}
	if id == "" {
		id = uuid.New().String()
	}
	msg := memory.Message{ID: id, ParentID: parentId, Role: role, Actor: actor, Content: content, ConversationID: convID}
	err := s.history.AddMessage(ctx, msg)
	return msg.ID, err
}

// conversationContext summarises the conversation according to configured
// policies and returns a plain-text string.
func (s *Service) conversationContext(ctx context.Context, convID string, qi *QueryInput) (string, error) {
	summarizerFn := summary.Build(s.llm, qi.Agent.Model, s.summaryPrompt, convID)
	policy := memory.NewCombinedPolicy(
		memory.NewSummaryPolicy(s.summaryThreshold, summarizerFn),
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
func (s *Service) runWorkflow(ctx context.Context, qi *QueryInput, qo *QueryOutput, convID, query string) error {
	// 1. Retrieve documents for enrichment.
	//    When the current user query is empty, fall back to previous
	//    conversation context so that vector search still has a hint.
	var searchInput = *qi // shallow copy
	if strings.TrimSpace(searchInput.Query) == "" {
		searchInput.Query = query
	}

	docs, err := s.retrieveRelevantDocuments(ctx, &searchInput)
	if err != nil {
		return fmt.Errorf("failed to retrieve knowledge: %w", err)
	}
	qo.Documents = docs

	// If agent has no tools AND no documents were found, running the workflow
	// would only waste tokens – fall back to direct answer.
	if len(qi.Agent.Tool) == 0 && len(docs) == 0 {
		return s.directAnswer(ctx, qi, qo, convID, query)
	}

	enrichment := s.buildEnrichment(query, s.formatDocumentsForEnrichment(docs, qi.IncludeFile), qi.Context)

	// 2. System prompt from agent template.
	sysPrompt, err := s.buildSystemPrompt(ctx, qi, enrichment)
	if err != nil {
		return err
	}
	wf, initial, err := s.loadWorkflow(ctx, qi, enrichment, sysPrompt)
	if err != nil {
		return err
	}
	// inject policy mode
	if p := tool.FromContext(ctx); p != nil {
		initial[keyToolPolicy] = p.Mode
	}
	// 3. Run workflow – carry conversation ID on context so that downstream
	// services (exec/run_plan) can record execution traces.
	ctx = context.WithValue(ctx, memory.ConversationIDKey, convID)
	_, wait, err := s.runtime.StartProcess(ctx, wf, initial)
	if err != nil {
		return fmt.Errorf("workflow start error: %w", err)
	}

	result, err := wait(ctx, s.workflowTimeout)
	if err != nil {
		return fmt.Errorf("workflow execution error: %w", err)
	}
	// -----------------------------------------------------------------
	// When some workflow steps report errors we do *not* immediately abort
	// the reasoning process.  The agent keeps going as long as at least one
	// tool invocation produced a usable output (answer / elicitation).  If
	// every step failed (i.e. answer & elicitation missing) we fall back to a
	// partial response so that the caller still receives diagnostic details.
	// -----------------------------------------------------------------
	if len(result.Errors) > 0 {
		if _, hasAnswer := result.Output[keyAnswer]; !hasAnswer {
			if _, hasElicit := result.Output[keyElicitation]; !hasElicit {
				// No successful output – surface diagnostic summary instead of
				// bubbling a hard error.
				errsJSON, _ := json.Marshal(result.Errors)
				var resSummary string
				if resRaw, ok := result.Output[keyResults]; ok {
					if b, err := json.Marshal(resRaw); err == nil {
						resSummary = string(b)
					}
				}

				qo.Content = fmt.Sprintf("I encountered an internal issue while composing the answer (details: %s).\n\nHere are the raw tool results I managed to obtain:\n%s", string(errsJSON), resSummary)
				qo.DocumentsSize = s.calculateDocumentsSize(docs)
				s.recordAssistant(ctx, convID, qo.Content, qi.Persona, qi.Agent.Name)
				qo.Usage = usage.FromContext(ctx)
				return nil
			}
		}
	}

	// 4. Extract output – same logic but isolated.

	// Always attempt to capture the plan/refinedPlan (if present) so that UI or callers
	// can render the current execution strategy irrespective of the final outcome.
	if planVal, ok := result.Output[keyRefinedPlan]; ok && planVal != nil {
		if p, err := coercePlan(planVal); err == nil {
			qo.Plan = p
		}
	} else if planVal, ok := result.Output[keyPlan]; ok && planVal != nil {
		if p, err := coercePlan(planVal); err == nil {
			qo.Plan = p
		}
	}

	if elVal, ok := result.Output[keyElicitation]; ok && elVal != nil {
		if elic, err := coerceElicitation(elVal); err == nil && elic != nil {
			qo.Elicitation = elic
			qo.Content = elic.Message
			qo.DocumentsSize = s.calculateDocumentsSize(docs)
			s.recordAssistantElicitation(ctx, convID, "", elic)
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
	// When orchestration produced a plain JSON elicitation block instead of
	// using the dedicated output field we still want interactive prompting.
	if qo.Elicitation == nil {
		if elic, ok := detectInlineElicitation(qo.Content); ok {
			qo.Elicitation = elic
			qo.Content = elic.Message
		}
	}
	qo.DocumentsSize = s.calculateDocumentsSize(docs)
	s.recordAssistant(ctx, convID, qo.Content, qi.Persona, qi.Agent.Name)

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

func (s *Service) recordAssistant(ctx context.Context, convID, content string, persona *agentmdl.Persona, defaultActor string) {
	parentID := memory.MessageIDFromContext(ctx)
	role := "assistant"
	actor := defaultActor
	if persona != nil {
		if persona.Role != "" {
			role = persona.Role
		}
		if persona.Actor != "" {
			actor = persona.Actor
		}
	}
	if _, err := s.addMessage(ctx, convID, role, actor, content, uuid.New().String(), parentID); err != nil {
		log.Printf("warn: cannot record assistant message: %v", err)
	}
}

// recordAssistantElicitation stores an assistant message that carries a
// structured schema-based elicitation. The message is flagged with the
// Elicitation field so that REST callers (and consequently the Forge UI)
// receive the full schema and can render an interactive form.
func (s *Service) recordAssistantElicitation(ctx context.Context, convID string, messageID string, elic *plan.Elicitation) {
	if elic == nil {
		return
	}

	// Refine schema for better UX.
	refiner.Refine(&elic.RequestedSchema)
	parentID := memory.MessageIDFromContext(ctx)
	if messageID != "" {
		parentID = messageID
	}
	msg := memory.Message{
		ID:             uuid.New().String(),
		ParentID:       parentID,
		ConversationID: convID,
		Role:           "assistant",
		Content:        elic.Message,
		Elicitation:    elic,
		CreatedAt:      time.Now(),
	}
	if err := s.history.AddMessage(ctx, msg); err != nil {
		log.Printf("warn: cannot record elicitation message: %v", err)
	}
}

// buildEnrichment merges conversation context and knowledge enrichment.
func (s *Service) buildEnrichment(conv, docs string, context map[string]interface{}) string {
	parts := []string{}
	if conv != "" {
		parts = append(parts, "Conversation:\n"+conv)
	}
	if docs != "" {
		parts = append(parts, "Documents:\n"+docs)
	}
	for k, v := range context {
		parts = append(parts, fmt.Sprintf("%s: %v", k, v))
	}
	return strings.Join(parts, "\n\n")
}

//go:embed orchestration/workflow.yaml
var orchestrationWorkflow []byte

func (s *Service) loadWorkflow(ctx context.Context, qi *QueryInput, enrichment, systemPrompt string) (*model.Workflow, map[string]interface{}, error) {
	toolNames, err := s.ensureTools(qi)
	if err != nil {
		return nil, nil, err
	}

	var wf *model.Workflow
	if URI := qi.Agent.OrchestrationFlow; strings.TrimSpace(URI) != "" {
		URI = s.ensureLocation(ctx, qi.Agent.Source, URI)
		wf, err = s.runtime.LoadWorkflow(ctx, URI)
	} else {
		wf, err = s.runtime.DecodeYAMLWorkflow(orchestrationWorkflow)
	}
	aModel := qi.Agent.Model
	if qi.ModelOverride != "" {
		aModel = qi.ModelOverride
	}
	initial := map[string]interface{}{
		keyQuery:        qi.Query,
		keyContext:      enrichment,
		keyModel:        aModel,
		keyTools:        toolNames,
		keySystemPrompt: systemPrompt,
	}

	return wf, initial, err
}

func (s *Service) ensureLocation(ctx context.Context, parent *agentmdl.Source, URI string) string {
	if parent == nil || parent.URL == "" || !url.IsRelative(URI) {
		return URI
	}
	if ok, _ := s.fs.Exists(ctx, URI); ok {
		return URI
	}

	parentURI, _ := url.Split(parent.URL, file.Scheme)
	if url.IsRelative(parent.URL) {
		parentURI, _ = path.Split(parent.URL)
	}
	URI = url.Join(parentURI, URI)
	if ok, _ := s.fs.Exists(ctx, URI); ok {
		return URI
	}
	URI = url.Join(workspace.Root(), URI)
	return URI
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

	model := qi.Agent.Model
	if qi.ModelOverride != "" {
		model = qi.ModelOverride
	}
	genIn := &corepkg.GenerateInput{
		Model:        model,
		SystemPrompt: sysPrompt,
		Prompt:       qi.Query,
		Options:      &llm.Options{Temperature: qi.Agent.Temperature},
	}
	genOut := &corepkg.GenerateOutput{}
	if err := exec(ctx, genIn, genOut); err != nil {
		return err
	}

	qo.Content = genOut.Content
	qo.Model = model

	// ------------------------------------------------------------
	// Check if the LLM response is an inline elicitation payload
	// (i.e. JSON object with "type":"elicitation"). When detected,
	// populate qo.Elicitation so that downstream services can invoke
	// the interactive Awaiter instead of printing raw JSON.
	// ------------------------------------------------------------
	if elic, ok := detectInlineElicitation(genOut.Content); ok {
		qo.Elicitation = elic
		// Use the human-readable message for the transcript instead of
		// the raw JSON block.
		qo.Content = elic.Message
	}

	s.recordAssistant(ctx, convID, qo.Content, qi.Persona, qi.Agent.Name)

	qo.Usage = usage.FromContext(ctx)
	return nil
}

// detectInlineElicitation tries to interpret text as a JSON document of the
// form {"type":"elicitation", ...}. It tolerates Markdown code fences.
func detectInlineElicitation(text string) (*plan.Elicitation, bool) {
	text = strings.TrimSpace(text)
	if text == "" || !strings.Contains(text, "\"type\"") {
		return nil, false
	}

	// Remove possible ```json code fences
	if strings.HasPrefix(text, "```") {
		if idx := strings.Index(text, "\n"); idx != -1 {
			text = text[idx+1:]
		}
		if end := strings.LastIndex(text, "```"); end != -1 {
			text = text[:end]
		}
		text = strings.TrimSpace(text)
	}

	if !strings.HasPrefix(text, "{") {
		return nil, false
	}

	// Probe for the type field first so we avoid unmarshalling unrelated data
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(text), &probe); err != nil {
		return nil, false
	}
	if strings.ToLower(probe.Type) != "elicitation" {
		return nil, false
	}

	var elic plan.Elicitation
	if err := json.Unmarshal([]byte(text), &elic); err != nil {
		return nil, false
	}
	return &elic, true
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
	// map[interface{}]interface{} handled above
	case map[interface{}]interface{}:
		conv := make(map[string]interface{}, len(actual))
		for k, v := range actual {
			if strKey, ok := k.(string); ok {
				conv[strKey] = v
			}
		}
		data, _ := json.Marshal(conv)
		return unmarshal(data)
	default:
		return nil, fmt.Errorf("unsupported elicitation type %T", v)
	}
}

// coercePlan converts various representations found in workflow output
// into *plan.Plan.
func coercePlan(v interface{}) (*plan.Plan, error) {
	unmarshal := func(data []byte) (*plan.Plan, error) {
		var p plan.Plan
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		return &p, nil
	}

	switch actual := v.(type) {
	case *plan.Plan:
		return actual, nil
	case plan.Plan:
		return &actual, nil
	case string:
		return unmarshal([]byte(actual))
	case []byte:
		return unmarshal(actual)
	case map[string]interface{}:
		data, _ := json.Marshal(actual)
		return unmarshal(data)
	default:
		// unsupported type
		return nil, fmt.Errorf("unexpected plan type %T", v)
	}
}
