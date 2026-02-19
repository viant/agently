package agent

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/viant/afs"
	"github.com/viant/afs/url"
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/elicitation"
	"github.com/viant/agently/genai/llm"
	base "github.com/viant/agently/genai/llm/provider/base"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/prompt"
	padapter "github.com/viant/agently/genai/prompt/adapter"
	"github.com/viant/agently/genai/service/core"
	executil "github.com/viant/agently/genai/service/shared/executil"
	"github.com/viant/agently/internal/workspace"
	"github.com/viant/agently/internal/workspace/repository/toolplaybook"
	mcpname "github.com/viant/agently/pkg/mcpname"

	intmodel "github.com/viant/agently/internal/finder/model"
)

func (s *Service) BuildBinding(ctx context.Context, input *QueryInput) (*prompt.Binding, error) {
	start := time.Now()
	convoID := ""
	if input != nil {
		convoID = strings.TrimSpace(input.ConversationID)
	}
	debugf("agent.BuildBinding start convo=%q", convoID)
	b := &prompt.Binding{}
	if input.Agent != nil {
		b.Model = input.Agent.Model
	}
	if strings.TrimSpace(input.ModelOverride) != "" {
		b.Model = strings.TrimSpace(input.ModelOverride)
	}
	// Fetch conversation transcript once and reuse; bubble up errors
	if s.conversation == nil {
		debugf("agent.BuildBinding error convo=%q elapsed=%s err=%v", convoID, time.Since(start).String(), "conversation API not configured")
		return nil, fmt.Errorf("conversation API not configured")
	}
	fetchStart := time.Now()
	debugf("agent.BuildBinding fetchConversation start convo=%q", convoID)
	conv, err := s.fetchConversationWithRetry(ctx, input.ConversationID, apiconv.WithIncludeToolCall(true))
	if err != nil {
		debugf("agent.BuildBinding fetchConversation error convo=%q elapsed=%s err=%v", convoID, time.Since(fetchStart).String(), err)
		return nil, err
	}
	debugf("agent.BuildBinding fetchConversation ok convo=%q elapsed=%s", convoID, time.Since(fetchStart).String())
	if conv == nil {
		debugf("agent.BuildBinding error convo=%q elapsed=%s err=%v", convoID, time.Since(start).String(), "conversation not found")
		return nil, fmt.Errorf("conversation not found: %s", strings.TrimSpace(input.ConversationID))
	}

	// Compute effective preview limit using service defaults only
	histStart := time.Now()
	debugf("agent.BuildBinding buildHistory start convo=%q", convoID)
	hist, elicitation, histOverflow, maxHistOverflowBytes, err := s.buildHistoryWithLimit(ctx, conv.GetTranscript(), input)
	if err != nil {
		debugf("agent.BuildBinding buildHistory error convo=%q elapsed=%s err=%v", convoID, time.Since(histStart).String(), err)
		return nil, err
	}
	debugf("agent.BuildBinding buildHistory ok convo=%q elapsed=%s overflow=%t elicitation=%d", convoID, time.Since(histStart).String(), histOverflow, len(elicitation))
	b.History = hist
	// Align History.CurrentTurnID with the in-flight turn when available
	if tm, ok := memory.TurnMetaFromContext(ctx); ok {
		b.History.CurrentTurnID = strings.TrimSpace(tm.TurnID)
	}
	// Attach current-turn elicitation messages to History.Current so
	// they can participate in a unified, chronological view of the
	// in-flight turn.
	if len(elicitation) > 0 {
		appendCurrentMessages(&b.History, elicitation...)
	}
	// Populate History.LastResponse using the last assistant message in transcript
	if conv != nil {
		tr := conv.GetTranscript()
		if last := tr.LastAssistantMessageWithModelCall(); last != nil {
			trace := &prompt.Trace{At: last.CreatedAt, Kind: prompt.KindResponse}
			if last.ModelCall != nil && last.ModelCall.TraceId != nil {
				if id := strings.TrimSpace(*last.ModelCall.TraceId); id != "" {
					trace.ID = id
				}
			}
			b.History.LastResponse = trace
			// Build History.Traces map: resp, opid and content keys
			b.History.Traces = s.buildTraces(tr)

		}
	}
	// Elicitation payload is merged into binding.Context later (after cloning input.Context)
	// to avoid mutating caller-supplied maps and to keep a single authoritative merge point.
	if histOverflow {
		b.Flags.HasMessageOverflow = true
	}
	if maxHistOverflowBytes > 0 {
		b.Flags.MaxOverflowBytes = maxHistOverflowBytes
	}

	b.Task = s.buildTaskBinding(input)

	toolsStart := time.Now()
	debugf("agent.BuildBinding buildToolSignatures start convo=%q", convoID)
	b.Tools.Signatures, _, err = s.buildToolSignatures(ctx, input)
	if err != nil {
		debugf("agent.BuildBinding buildToolSignatures error convo=%q elapsed=%s err=%v", convoID, time.Since(toolsStart).String(), err)
		return nil, err
	}
	debugf("agent.BuildBinding buildToolSignatures ok convo=%q elapsed=%s tools=%d", convoID, time.Since(toolsStart).String(), len(b.Tools.Signatures))

	// Tool executions exposure: default "turn"; allow QueryInput override; then agent setting.
	exposure := agent.ToolCallExposure("turn")
	if input.ToolCallExposure != nil && strings.TrimSpace(string(*input.ToolCallExposure)) != "" {
		exposure = *input.ToolCallExposure
	} else if input.Agent != nil && strings.TrimSpace(string(input.Agent.Tool.CallExposure)) != "" {
		exposure = input.Agent.Tool.CallExposure
	}

	b.History.ToolExposure = string(exposure)

	execStart := time.Now()
	debugf("agent.BuildBinding buildToolExecutions start convo=%q exposure=%q", convoID, string(exposure))
	_, overflow, maxExecOverflowBytes, err := s.buildToolExecutions(ctx, input, conv, exposure)
	if err != nil {
		debugf("agent.BuildBinding buildToolExecutions error convo=%q elapsed=%s err=%v", convoID, time.Since(execStart).String(), err)
		return nil, err
	}
	debugf("agent.BuildBinding buildToolExecutions ok convo=%q elapsed=%s overflow=%t", convoID, time.Since(execStart).String(), overflow)

	// Drive overflow-based helper exposure via binding flag
	if overflow {
		b.Flags.HasMessageOverflow = true
	}
	if maxExecOverflowBytes > 0 && maxExecOverflowBytes > b.Flags.MaxOverflowBytes {
		b.Flags.MaxOverflowBytes = maxExecOverflowBytes
	}

	// If any tool call in the current turn overflowed, expose callToolResult tools
	turnMeta, ok := memory.TurnMetaFromContext(ctx)
	if ok && strings.TrimSpace(turnMeta.TurnID) != "" {
		var current *apiconv.Turn
		for _, t := range conv.GetTranscript() {
			if t != nil && t.Id == turnMeta.TurnID {
				current = t
				break
			}
		}
		s.handleOverflow(ctx, input, current, b)
		// Allow tool-use if we appended any
		if len(b.Tools.Signatures) > 0 && b.Model != "" {
			b.Flags.CanUseTool = s.llm.ModelImplements(ctx, b.Model, base.CanUseTools)
		}

	}

	// Append internal tools needed for continuation flows (no duplicates)
	s.ensureInternalToolsIfNeeded(ctx, input, b)

	docsStart := time.Now()
	debugf("agent.BuildBinding buildDocuments start convo=%q", convoID)
	docs, err := s.buildDocumentsBinding(ctx, input, false)
	if err != nil {
		debugf("agent.BuildBinding buildDocuments error convo=%q elapsed=%s err=%v", convoID, time.Since(docsStart).String(), err)
		return nil, err
	}

	debugf("agent.BuildBinding buildDocuments ok convo=%q elapsed=%s docs=%d", convoID, time.Since(docsStart).String(), len(docs.Items))
	b.Documents = docs
	// Normalize user doc URIs by trimming workspace root for stable display
	s.normalizeDocURIs(&b.Documents, workspace.Root())
	// Attach non-text user documents as binary attachments (e.g., PDFs, images)
	s.attachNonTextUserDocs(ctx, b)

	sysDocsStart := time.Now()
	debugf("agent.BuildBinding buildSystemDocuments start convo=%q", convoID)
	b.SystemDocuments, err = s.buildDocumentsBinding(ctx, input, true)
	if err != nil {
		debugf("agent.BuildBinding buildSystemDocuments error convo=%q elapsed=%s err=%v", convoID, time.Since(sysDocsStart).String(), err)
		return nil, err
	}
	debugf("agent.BuildBinding buildSystemDocuments ok convo=%q elapsed=%s docs=%d", convoID, time.Since(sysDocsStart).String(), len(b.SystemDocuments.Items))
	s.appendTranscriptSystemDocs(conv.GetTranscript(), b)
	if err := s.appendToolPlaybooks(ctx, b.Tools.Signatures, &b.SystemDocuments); err != nil {
		debugf("agent.BuildBinding appendToolPlaybooks error convo=%q elapsed=%s err=%v", convoID, time.Since(sysDocsStart).String(), err)
		return nil, err
	}
	s.appendAgentDirectoryDoc(ctx, input, &b.SystemDocuments)
	// Normalize system doc URIs similarly (even if not rendered now)
	s.normalizeDocURIs(&b.SystemDocuments, workspace.Root())
	b.Context = input.Context

	// Expose tool availability flags for templates (dynamic tool selection).
	// Avoid mutating input.Context directly by working on a copy.
	b.Context = cloneContextMap(b.Context)
	mergeElicitationPayloadIntoContext(b.History, &b.Context)
	applyToolContext(b.Context, b.Tools.Signatures)
	s.applyDelegationContext(input, b)

	debugf("agent.BuildBinding ok convo=%q elapsed=%s history_msgs=%d sys_docs=%d docs=%d tools=%d", convoID, time.Since(start).String(), len(b.History.Messages), len(b.SystemDocuments.Items), len(b.Documents.Items), len(b.Tools.Signatures))
	return b, nil
}

func cloneContextMap(src map[string]interface{}) map[string]interface{} {
	if len(src) == 0 {
		return map[string]interface{}{}
	}
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func (s *Service) applyDelegationContext(input *QueryInput, b *prompt.Binding) {
	if input == nil || b == nil || input.Agent == nil {
		debugf("delegation.context skip missing input/binding/agent")
		return
	}
	if input.Agent.Delegation == nil || !input.Agent.Delegation.Enabled {
		debugf("delegation.context disabled agent_id=%q", strings.TrimSpace(input.Agent.ID))
		return
	}
	if b.Context == nil {
		b.Context = map[string]interface{}{}
	}
	maxDepth := input.Agent.Delegation.MaxDepth
	if maxDepth <= 0 {
		maxDepth = 2
	}
	b.Context["DelegationEnabled"] = true
	b.Context["DelegationMaxDepth"] = maxDepth
	if strings.TrimSpace(input.Agent.ID) != "" {
		b.Context["DelegationSelfID"] = strings.TrimSpace(input.Agent.ID)
	}
	// Seed a depth map if missing so templates can reference it.
	if _, ok := b.Context["DelegationDepths"]; !ok {
		b.Context["DelegationDepths"] = map[string]interface{}{}
	}
	debugf("delegation.context enabled agent_id=%q maxDepth=%d", strings.TrimSpace(input.Agent.ID), maxDepth)
}

func (s *Service) appendAgentDirectoryDoc(ctx context.Context, input *QueryInput, docs *prompt.Documents) {
	if s == nil || s.registry == nil || input == nil || input.Agent == nil || docs == nil {
		debugf("delegation.directory skip missing service/registry/input/agent/docs")
		return
	}
	if input.Agent.Delegation == nil || !input.Agent.Delegation.Enabled {
		debugf("delegation.directory disabled agent_id=%q", strings.TrimSpace(input.Agent.ID))
		return
	}
	// Avoid duplicate injection.
	const sourceURI = "internal://llm/agents/list"
	if hasDocumentURI(docs.Items, sourceURI) {
		debugf("delegation.directory skip already_present agent_id=%q", strings.TrimSpace(input.Agent.ID))
		return
	}
	raw, err := s.registry.Execute(ctx, "llm/agents:list", map[string]interface{}{})
	if err != nil {
		debugf("delegation.directory list_error agent_id=%q err=%v", strings.TrimSpace(input.Agent.ID), err)
		return
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		debugf("delegation.directory list_empty agent_id=%q", strings.TrimSpace(input.Agent.ID))
		return
	}
	var lo struct {
		Items []struct {
			ID          string `json:"id"`
			Name        string `json:"name,omitempty"`
			Description string `json:"description,omitempty"`
			Summary     string `json:"summary,omitempty"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &lo); err != nil || len(lo.Items) == 0 {
		if err != nil {
			debugf("delegation.directory list_unmarshal_error agent_id=%q err=%v", strings.TrimSpace(input.Agent.ID), err)
		} else {
			debugf("delegation.directory list_unmarshal_empty agent_id=%q", strings.TrimSpace(input.Agent.ID))
		}
		return
	}
	var bld strings.Builder
	bld.WriteString("# Available Agents\n\n")
	for _, it := range lo.Items {
		name := strings.TrimSpace(it.Name)
		if name == "" {
			name = strings.TrimSpace(it.ID)
		}
		if name == "" {
			continue
		}
		desc := strings.TrimSpace(it.Description)
		if desc == "" {
			desc = strings.TrimSpace(it.Summary)
		}
		bld.WriteString("- ")
		bld.WriteString(name)
		if id := strings.TrimSpace(it.ID); id != "" && id != name {
			bld.WriteString(" (`")
			bld.WriteString(id)
			bld.WriteString("`)")
		}
		if desc != "" {
			bld.WriteString(": ")
			bld.WriteString(desc)
		}
		bld.WriteString("\n")
	}
	doc := &prompt.Document{
		Title:       "agents/directory",
		PageContent: strings.TrimSpace(bld.String()),
		SourceURI:   sourceURI,
		MimeType:    "text/markdown",
		Metadata:    map[string]string{"kind": "agents_directory"},
	}
	docs.Items = append(docs.Items, doc)
	debugf("delegation.directory injected agent_id=%q count=%d", strings.TrimSpace(input.Agent.ID), len(lo.Items))
}

func (s *Service) appendToolPlaybooks(ctx context.Context, defs []*llm.ToolDefinition, docs *prompt.Documents) error {
	if docs == nil {
		return nil
	}
	_, services := collectToolPresence(defs)
	if !services["webdriver"] {
		return nil
	}
	repo := toolplaybook.New(s.fs)
	content, uri, err := repo.Load(ctx, "webdriver.md")
	if err != nil {
		return err
	}
	if strings.TrimSpace(content) == "" || strings.TrimSpace(uri) == "" {
		return nil
	}
	if hasDocumentURI(docs.Items, uri) {
		return nil
	}
	doc := &prompt.Document{
		Title:       "tools/hints/webdriver",
		PageContent: strings.TrimSpace(content),
		SourceURI:   uri,
		Score:       1.0,
		MimeType:    "text/markdown",
		Metadata:    map[string]string{"kind": "tool_playbook", "tool": "webdriver"},
	}
	docs.Items = append(docs.Items, doc)
	return nil
}

func hasDocumentURI(items []*prompt.Document, uri string) bool {
	u := strings.TrimSpace(uri)
	if u == "" || len(items) == 0 {
		return false
	}
	for _, d := range items {
		if d == nil {
			continue
		}
		if strings.TrimSpace(d.SourceURI) == u {
			return true
		}
	}
	return false
}

func applyToolContext(ctx map[string]interface{}, defs []*llm.ToolDefinition) {
	if ctx == nil {
		return
	}
	toolsCtx := ensureToolsContextMap(ctx)
	presentSet, serviceSet := collectToolPresence(defs)
	present := make(map[string]interface{}, len(presentSet))
	services := make(map[string]interface{}, len(serviceSet))
	for k, v := range presentSet {
		if v {
			present[k] = true
		}
	}
	for k, v := range serviceSet {
		if v {
			services[k] = true
		}
	}

	toolsCtx["present"] = present
	toolsCtx["services"] = services
	toolsCtx["hasWebdriver"] = serviceSet["webdriver"]
	toolsCtx["hasResources"] = serviceSet["resources"]
}

func collectToolPresence(defs []*llm.ToolDefinition) (map[string]bool, map[string]bool) {
	present := map[string]bool{}
	services := map[string]bool{}
	for _, d := range defs {
		if d == nil {
			continue
		}
		raw := strings.TrimSpace(d.Name)
		if raw == "" {
			continue
		}
		name := mcpname.Canonical(raw)
		if strings.TrimSpace(name) == "" {
			name = raw
		}
		present[name] = true
		svc := mcpname.Name(name).Service()
		if strings.TrimSpace(svc) != "" {
			services[svc] = true
		}
	}
	return present, services
}

func ensureToolsContextMap(ctx map[string]interface{}) map[string]interface{} {
	if ctx == nil {
		return map[string]interface{}{}
	}
	if v, ok := ctx["tools"]; ok && v != nil {
		if m, ok := v.(map[string]interface{}); ok {
			return m
		}
		// Preserve existing "tools" key when not an object.
		if v2, ok := ctx["agentlyTools"]; ok && v2 != nil {
			if m, ok := v2.(map[string]interface{}); ok {
				return m
			}
		}
		m := map[string]interface{}{}
		ctx["agentlyTools"] = m
		return m
	}
	m := map[string]interface{}{}
	ctx["tools"] = m
	return m
}

func (s *Service) buildTraces(tr apiconv.Transcript) map[string]*prompt.Trace {
	var result = make(map[string]*prompt.Trace)
	for _, turn := range tr {
		if turn == nil {
			continue
		}
		for _, m := range turn.GetMessages() {
			if m == nil {
				continue
			}
			// Assistant model-call response
			if m.ModelCall != nil && m.ModelCall.TraceId != nil {
				id := strings.TrimSpace(*m.ModelCall.TraceId)
				if id != "" {
					key := prompt.KindResponse.Key(id)
					result[key] = &prompt.Trace{ID: id, Kind: prompt.KindResponse, At: m.CreatedAt}
				}
				continue
			}
			// Tool-call message
			if m.ToolCall != nil {
				opID := strings.TrimSpace(m.ToolCall.OpId)
				if opID != "" {
					respId := ""
					if m.ToolCall.TraceId != nil {
						respId = strings.TrimSpace(*m.ToolCall.TraceId)
					}
					key := prompt.KindToolCall.Key(opID)
					result[key] = &prompt.Trace{ID: respId, Kind: prompt.KindToolCall, At: m.CreatedAt}
				}
				continue
			}

			// User/assistant text message
			if strings.ToLower(strings.TrimSpace(m.Type)) == "text" && m.Content != nil && *m.Content != "" {
				ckey := prompt.KindContent.Key(*m.Content)
				// Use a stable "effective at" timestamp for queued turns:
				// queued user messages may be persisted before the prior assistant
				// response exists, so comparing raw message.CreatedAt to the anchor
				// can incorrectly exclude the current prompt from continuation.
				// Prefer the later of turn.CreatedAt and message.CreatedAt.
				at := m.CreatedAt
				if !turn.CreatedAt.IsZero() && turn.CreatedAt.After(at) {
					at = turn.CreatedAt
				}
				result[ckey] = &prompt.Trace{ID: ckey, Kind: prompt.KindContent, At: at}
			}
		}
	}
	return result
}

// mergeElicitationPayloadIntoContext folds the most recent JSON object
// payloads from user elicitation messages into the binding context so
// downstream plans can see resolved inputs (e.g., workdir). Later
// messages win on key collision.
func mergeElicitationPayloadIntoContext(h prompt.History, ctxPtr *map[string]interface{}) {
	if ctxPtr == nil {
		return
	}
	if *ctxPtr == nil {
		*ctxPtr = map[string]interface{}{}
	}
	ctx := *ctxPtr

	// Helper to process a slice of messages in order.
	consume := func(msgs []*prompt.Message) {
		for _, m := range msgs {
			if m == nil || m.Kind != prompt.MessageKindElicitAnswer {
				continue
			}
			raw := strings.TrimSpace(m.Content)
			if raw == "" || !strings.HasPrefix(raw, "{") {
				continue
			}
			var payload map[string]interface{}
			if err := json.Unmarshal([]byte(raw), &payload); err != nil || len(payload) == 0 {
				continue
			}
			if elicitation.DebugEnabled() {
				log.Printf("[debug][elicitation] merge payloadKeys=%v", elicitation.PayloadKeys(payload))
			}
			for k, v := range payload {
				ctx[k] = v
			}
			// Lightweight alias normalization for common elicitation synonyms.
			// Helps when LLM varies field names across successive elicitations.
			if _, ok := ctx["favoriteColor"]; !ok {
				if v, ok := firstValue(payload, "favoriteColor", "favorite_color", "favColor", "fav_color", "color"); ok {
					ctx["favoriteColor"] = v
				}
			}
			if _, ok := ctx["color"]; !ok {
				if v, ok := firstValue(payload, "color", "favoriteColor", "favorite_color", "favColor", "fav_color"); ok {
					ctx["color"] = v
				}
			}
			if _, ok := ctx["shade"]; !ok {
				if v, ok := firstValue(payload, "shade", "shadeOrVariant", "variant"); ok {
					ctx["shade"] = v
				}
			}
			if _, ok := ctx["detailLevel"]; !ok {
				if v, ok := firstValue(payload, "detailLevel", "detail_level", "style", "tone", "toneOrVibe", "vibe"); ok {
					ctx["detailLevel"] = v
				}
			}
			if _, ok := ctx["style"]; !ok {
				if v, ok := firstValue(payload, "style", "detailLevel", "detail_level", "tone", "toneOrVibe", "vibe"); ok {
					ctx["style"] = v
				}
			}
			if _, ok := ctx["descriptionStyle"]; !ok {
				if v, ok := firstValue(payload, "descriptionStyle", "description_style", "style", "detailLevel", "detail_level", "tone", "toneOrVibe", "vibe"); ok {
					ctx["descriptionStyle"] = v
				}
			}
			if elicitation.DebugEnabled() {
				log.Printf("[debug][elicitation] ctxKeys=%v", elicitation.PayloadKeys(ctx))
			}
		}
	}

	for _, t := range h.Past {
		if t == nil {
			continue
		}
		consume(t.Messages)
	}
	if h.Current != nil {
		consume(h.Current.Messages)
	}
}

func firstValue(payload map[string]interface{}, keys ...string) (interface{}, bool) {
	if len(payload) == 0 {
		return nil, false
	}
	lower := map[string]interface{}{}
	for k, v := range payload {
		lower[strings.ToLower(strings.TrimSpace(k))] = v
	}
	for _, k := range keys {
		kk := strings.ToLower(strings.TrimSpace(k))
		if v, ok := lower[kk]; ok {
			return v, true
		}
	}
	return nil, false
}

// fetchConversationWithRetry attempts to fetch a conversation up to three times,
// applying a short exponential backoff on transient errors. It returns an error
// when the conversation is missing or on non-transient failures.
func (s *Service) fetchConversationWithRetry(ctx context.Context, id string, options ...apiconv.Option) (*apiconv.Conversation, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		conv, err := s.conversation.GetConversation(ctx, id, options...)
		if err == nil {
			if conv == nil {
				lastErr = fmt.Errorf("conversation not found: %s", strings.TrimSpace(id))
				break
			}
			return conv, nil
		}
		lastErr = err
		// Do not keep retrying if context is done
		if ctx.Err() != nil {
			break
		}
		if !isTransientDBOrNetworkError(err) || attempt == 2 {
			break
		}
		// 200ms, 400ms backoff (final attempt follows immediately)
		delay := 200 * time.Millisecond << attempt
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, fmt.Errorf("conversation fetch canceled: %w", err)
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("failed to fetch conversation: %w", lastErr)
	}
	return nil, fmt.Errorf("conversation not found: %s", strings.TrimSpace(id))
}

// isTransientDBOrNetworkError classifies intermittent DB/driver/network failures
// that are commonly resolved with a short retry.
func isTransientDBOrNetworkError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "timeout"),
		strings.Contains(msg, "i/o timeout"),
		strings.Contains(msg, "deadline exceeded"),
		strings.Contains(msg, "connection reset"),
		strings.Contains(msg, "connection refused"),
		strings.Contains(msg, "driver: bad connection"),
		strings.Contains(msg, "too many connections"),
		strings.Contains(msg, "server closed idle connection"),
		strings.Contains(msg, "deadlock"),
		strings.Contains(msg, "lock wait timeout"),
		strings.Contains(msg, "transaction aborted"),
		strings.Contains(msg, "temporary network error"),
		strings.Contains(msg, "network is unreachable"):
		return true
	}
	return false
}

func (s *Service) handleOverflow(ctx context.Context, input *QueryInput, current *apiconv.Turn, b *prompt.Binding) {
	// Detect token-limit recovery by scanning current turn for an assistant error message
	tokenLimit := false
	if current != nil && len(current.Message) > 0 {
		for _, m := range current.Message {
			if m == nil || m.Content == nil {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(m.Role), "assistant") && m.Status != nil && strings.EqualFold(strings.TrimSpace(*m.Status), "error") {
				{
					msg := strings.ToLower(*m.Content)
					if core.ContainsContextLimitError(msg) {
						tokenLimit = true
						break
					}
				}
			}
		}
	}
	// Drive from flags or token-limit hint
	if !b.Flags.HasMessageOverflow && !tokenLimit {
		return
	}

	// removed debug print: hasOverflow and existing signatures

	// Build a canonical set of already exposed tools to avoid duplicates
	have := map[string]bool{}
	for _, e := range b.Tools.Signatures {
		if e == nil {
			continue
		}
		have[mcpname.Canonical(e.Name)] = true
	}

	// Query only internal/message tools from the registry (avoid full scan)
	// Using a service-only pattern matches any method under that service.
	pattern := "internal/message" // service-only pattern (match any method)
	defs := s.registry.MatchDefinition(pattern)

	for _, d := range defs {
		if d == nil {
			continue
		}
		name := mcpname.Canonical(d.Name)
		// Only expose show/summarize/match on overflow; gate remove for token-limit flow
		// Derive method from tool name. Names can be in forms like:
		//   internal/message:show  (service:method)
		//   internal_message-show  (canonicalized with dash)
		// Fallback to full name when no separator present.
		method := ""
		if i := strings.LastIndexAny(name, ":-"); i != -1 && i+1 < len(name) {
			method = name[i+1:]
		}
		if method == "" {
			method = name
		}
		allowed := false
		if tokenLimit {
			if method == "remove" || method == "summarize" {
				allowed = true
			}
		} else if b.Flags.HasMessageOverflow {
			// For normal overflow, always expose show; gate summarize on
			// configured summaryThresholdBytes and recorded MaxOverflowBytes.
			if method == "show" || method == "match" {
				allowed = true
			} else if method == "summarize" {
				threshold := 0
				if s.defaults != nil {
					threshold = s.defaults.PreviewSettings.SummaryThresholdBytes
				}
				// When threshold <= 0, fallback to previous behavior and
				// allow summarize for any overflow. Otherwise, require at
				// least one overflowed message to exceed the threshold.
				if threshold <= 0 || b.Flags.MaxOverflowBytes > threshold {
					allowed = true
				}
			}
		}

		if !allowed {
			continue
		}
		if have[name] {
			continue
		}
		dd := *d
		// Canonicalize name to service_path-method for consistency (e.g., internal_message-match)
		dd.Name = mcpname.Canonical(dd.Name)
		b.Tools.Signatures = append(b.Tools.Signatures, &dd)
		have[name] = true
	}
	// removed debug print: final signatures count

	// Optionally append a system guide document when configured in defaults
	s.appendCallToolResultGuide(ctx, b)
}

func (s *Service) appendCallToolResultGuide(ctx context.Context, b *prompt.Binding) {
	if s.defaults != nil && strings.TrimSpace(s.defaults.PreviewSettings.SystemGuidePath) != "" {
		guide := strings.TrimSpace(s.defaults.PreviewSettings.SystemGuidePath)
		uri := guide
		if url.Scheme(uri, "") == "" {
			uri = "file://" + guide
		}
		if data, err := s.fs.DownloadWithURL(ctx, uri); err == nil && len(data) > 0 {
			title := filepath.Base(guide)
			if strings.TrimSpace(title) == "" {
				title = "Tool Result Guide"
			}
			doc := &prompt.Document{Title: title, PageContent: string(data), SourceURI: uri, MimeType: "text/markdown"}
			b.SystemDocuments.Items = append(b.SystemDocuments.Items, doc)
		}
	}
}

// ensureInternalToolsIfNeeded appends internal/message tools that are used during
// continuation-by-response-id flows so that the model can reference them when
// continuing a prior response. Tool are appended only when the selected model
// supports continuation. Duplicates are avoided by canonical name.
func (s *Service) ensureInternalToolsIfNeeded(ctx context.Context, input *QueryInput, b *prompt.Binding) {
	if s == nil || s.registry == nil || b == nil {
		return
	}
	modelName := strings.TrimSpace(b.Model)
	if modelName == "" {
		return
	}

	// Decide based on the same continuation semantics as the core service.
	finder := s.llm.ModelFinder()
	if finder == nil {
		return
	}
	model, err := finder.Find(ctx, modelName)
	if err != nil || model == nil {
		return
	}
	if !core.IsContextContinuationEnabled(model) {
		return
	}

	// Build set of existing tool names to avoid duplicates
	have := map[string]bool{}
	for _, sig := range b.Tools.Signatures {
		if sig == nil {
			continue
		}
		have[mcpname.Canonical(sig.Name)] = true
	}

	// Collect internal/message tool definitions and append a consistent subset used in overflow handling
	// We include: show, summarize, match, remove (the union of tools referenced in handleOverflow).
	defs := s.registry.MatchDefinition("internal/message")
	wanted := map[string]bool{"show": true, "summarize": true, "match": true, "remove": true}
	for _, d := range defs {
		if d == nil {
			continue
		}
		name := mcpname.Canonical(d.Name)
		// Derive method suffix
		method := name
		if i := strings.LastIndexAny(name, ":-"); i != -1 && i+1 < len(name) {
			method = name[i+1:]
		}
		if !wanted[method] {
			continue
		}
		if have[name] {
			continue
		}
		dd := *d
		dd.Name = name
		b.Tools.Signatures = append(b.Tools.Signatures, &dd)
		have[name] = true
	}
}

// normalizeDocURIs trims a prefix from document SourceURI for stable, short references
func (s *Service) normalizeDocURIs(docs *prompt.Documents, trim string) {
	if docs == nil || len(docs.Items) == 0 {
		return
	}
	trim = strings.TrimSpace(trim)
	if trim == "" {
		return
	}
	// Ensure trailing slash for precise trimming
	if !strings.HasSuffix(trim, "/") {
		trim += "/"
	}
	for _, d := range docs.Items {
		if d == nil {
			continue
		}
		uri := strings.TrimSpace(d.SourceURI)
		if uri == "" {
			continue
		}
		if strings.HasPrefix(uri, trim) {
			d.SourceURI = strings.TrimPrefix(uri, trim)
		}
	}
}

func (s *Service) appendTranscriptSystemDocs(tr apiconv.Transcript, b *prompt.Binding) {
	if b == nil {
		return
	}
	systemDocs := transcriptSystemDocuments(tr)
	if len(systemDocs) == 0 {
		return
	}
	if b.SystemDocuments.Items == nil {
		b.SystemDocuments.Items = []*prompt.Document{}
	}
	seen := map[string]bool{}
	contentHashes := map[string]bool{}
	for _, doc := range b.SystemDocuments.Items {
		if key := systemDocDedupKey(doc); key != "" {
			seen[key] = true
		}
		if hash := systemDocContentHash(doc); hash != "" {
			contentHashes[hash] = true
		}
	}
	for _, doc := range systemDocs {
		if doc == nil {
			continue
		}
		key := systemDocDedupKey(doc)
		if key != "" {
			if seen[key] {
				continue
			}
			seen[key] = true
		}
		if hash := systemDocContentHash(doc); hash != "" {
			if contentHashes[hash] {
				continue
			}
			contentHashes[hash] = true
		}
		b.SystemDocuments.Items = append(b.SystemDocuments.Items, doc)
	}
}

// allowContinuationPreview reports whether continuation preview formatting is
// enabled for the selected model on this turn. It resolves the effective model
// from QueryInput (ModelOverride > Agent.Model) and inspects the provider
// config option EnableContinuationFormat when available.
func (s *Service) allowContinuationPreview(ctx context.Context, input *QueryInput) bool {
	if s == nil || input == nil {
		return false
	}
	modelName := ""
	if strings.TrimSpace(input.ModelOverride) != "" {
		modelName = strings.TrimSpace(input.ModelOverride)
	} else if input.Agent != nil {
		modelName = strings.TrimSpace(input.Agent.Model)
	}
	if modelName == "" || s.llm == nil {
		return false
	}
	f := s.llm.ModelFinder()
	if f == nil {
		return false
	}
	if mf, ok := f.(*intmodel.Finder); ok {
		if cfg := mf.ConfigByIDOrModel(modelName); cfg != nil {
			return cfg.Options.EnableContinuationFormat
		}
	}
	return false
}

func transcriptSystemDocuments(tr apiconv.Transcript) []*prompt.Document {
	if len(tr) == 0 {
		return nil
	}
	var docs []*prompt.Document
	seen := map[string]bool{}
	for _, turn := range tr {
		if turn == nil || len(turn.GetMessages()) == 0 {
			continue
		}
		for _, msg := range turn.GetMessages() {
			doc := toSystemDocument(turn, msg)
			if doc == nil {
				continue
			}
			key := systemDocDedupKey(doc)
			if key != "" {
				if seen[key] {
					continue
				}
				seen[key] = true
			}
			docs = append(docs, doc)
		}
	}
	return docs
}

func toSystemDocument(turn *apiconv.Turn, msg *apiconv.Message) *prompt.Document {
	if msg == nil || !hasSystemDocTag(msg) {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(msg.Role), "system") {
		return nil
	}
	content := strings.TrimSpace(msg.GetContent())
	if content == "" {
		return nil
	}
	source := extractSystemDocSource(msg, content)
	meta := map[string]string{
		"messageId": strings.TrimSpace(msg.Id),
	}
	if turn != nil && strings.TrimSpace(turn.Id) != "" {
		meta["turnId"] = strings.TrimSpace(turn.Id)
	}
	if source != "" {
		meta["source"] = source
	}
	return &prompt.Document{
		Title:       deriveSystemDocTitle(source),
		PageContent: content,
		SourceURI:   source,
		MimeType:    inferMimeTypeFromSource(source),
		Metadata:    meta,
	}
}

func extractSystemDocSource(msg *apiconv.Message, content string) string {
	if msg != nil && msg.ContextSummary != nil {
		if v := strings.TrimSpace(*msg.ContextSummary); v != "" {
			return v
		}
	}
	firstLine := ""
	if content != "" {
		parts := strings.SplitN(content, "\n", 2)
		if len(parts) > 0 {
			firstLine = strings.TrimSpace(parts[0])
		}
	}
	if firstLine != "" && strings.HasPrefix(strings.ToLower(firstLine), "file:") {
		return strings.TrimSpace(firstLine[len("file:"):])
	}
	return ""
}

func deriveSystemDocTitle(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return "System Document"
	}
	base := strings.TrimSpace(path.Base(source))
	if base == "" || base == "." || base == "/" {
		return source
	}
	return base
}

func inferMimeTypeFromSource(source string) string {
	switch strings.ToLower(strings.TrimSpace(path.Ext(strings.TrimSpace(source)))) {
	case ".md", ".markdown":
		return "text/markdown"
	case ".json":
		return "application/json"
	case ".sql":
		return "text/plain"
	case ".go", ".py", ".ts", ".tsx", ".js", ".java", ".rb":
		return "text/plain"
	}
	return "text/markdown"
}

func hasSystemDocTag(msg *apiconv.Message) bool {
	if msg == nil {
		return false
	}
	if msg.Tags != nil {
		for _, tag := range strings.Split(*msg.Tags, ",") {
			if strings.EqualFold(strings.TrimSpace(tag), executil.SystemDocumentTag) {
				return true
			}
		}
	}
	mode := ""
	if msg.Mode != nil {
		mode = *msg.Mode
	}
	if strings.EqualFold(strings.TrimSpace(mode), executil.SystemDocumentMode) {
		return true
	}
	content := strings.ToLower(strings.TrimSpace(msg.GetContent()))
	return strings.HasPrefix(content, "file:")
}

func systemDocKey(doc *prompt.Document) string {
	if doc == nil {
		return ""
	}
	if v := strings.TrimSpace(doc.SourceURI); v != "" {
		return v
	}
	if doc.Metadata != nil {
		if id := strings.TrimSpace(doc.Metadata["messageId"]); id != "" {
			return id
		}
	}
	return strings.TrimSpace(doc.Title)
}

func systemDocDedupKey(doc *prompt.Document) string {
	if doc == nil {
		return ""
	}
	if key := systemDocKey(doc); key != "" {
		return key
	}
	title := strings.TrimSpace(doc.Title)
	content := strings.TrimSpace(doc.PageContent)
	if title == "" && content == "" {
		return ""
	}
	sum := md5.Sum([]byte(title + "::" + content))
	return hex.EncodeToString(sum[:])
}

func systemDocContentHash(doc *prompt.Document) string {
	if doc == nil {
		return ""
	}
	content := strings.TrimSpace(doc.PageContent)
	if content == "" {
		return ""
	}
	sum := md5.Sum([]byte(content))
	return hex.EncodeToString(sum[:])
}

// debugf prints binding-level debug logs when AGENTLY_DEBUG or AGENTLY_DEBUG_BINDING is set.
// debugf is intentionally a no-op to avoid noisy logs during normal operation.
// Keep the function for quick re‑enablement if needed while troubleshooting.

func extractSource(meta map[string]any) string {
	if meta == nil {
		return ""
	}
	if v, ok := meta["path"]; ok {
		if s, _ := v.(string); strings.TrimSpace(s) != "" {
			return s
		}
	}
	if v, ok := meta["docId"]; ok {
		if s, _ := v.(string); strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func baseName(uri string) string {
	if uri == "" {
		return ""
	}
	if b := path.Base(uri); b != "." && b != "/" {
		return b
	}
	return uri
}

// attachNonTextUserDocs scans user documents and adds non-text docs as attachments.
// It avoids duplicating content in user templates, which now render references only.
func (s *Service) attachNonTextUserDocs(ctx context.Context, b *prompt.Binding) {
	if b == nil || len(b.Documents.Items) == 0 {
		return
	}
	for _, d := range b.Documents.Items {
		uri := strings.TrimSpace(d.SourceURI)
		if uri == "" {
			continue
		}
		mime := mimeFromExt(strings.ToLower(strings.TrimPrefix(path.Ext(uri), ".")))
		if !isNonTextMime(mime) {
			continue
		}
		var data []byte
		// Skip MCP URIs for binding attachments; handled via resources tools on-demand
		if !strings.HasPrefix(strings.ToLower(uri), "mcp:") {
			if raw, err := s.fs.DownloadWithURL(ctx, uri); err == nil && len(raw) > 0 {
				data = raw
			}
		}
		if len(data) == 0 {
			continue
		}
		b.Task.Attachments = append(b.Task.Attachments, &prompt.Attachment{
			Name: baseName(uri), URI: uri, Mime: mime, Data: data,
		})
	}
}

func isNonTextMime(m string) bool {
	switch m {
	case "application/pdf", "image/png", "image/jpeg", "image/gif", "image/webp", "image/bmp", "image/svg+xml":
		return true
	}
	return false
}

func mimeFromExt(ext string) string {
	switch ext {
	case "pdf":
		return "application/pdf"
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	case "bmp":
		return "image/bmp"
	case "svg":
		return "image/svg+xml"
	default:
		return "text/plain"
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// fetchMCPResource resolves an mcp resource URI via MCP client.
// buildMCPDocuments and MCP fetch helpers removed — MCPResources retired.

func (s *Service) BuildHistory(ctx context.Context, transcript apiconv.Transcript, binding *prompt.Binding) error {
	hist, err := s.buildHistory(ctx, transcript)
	if err != nil {
		return err
	}
	binding.History = hist
	return nil
}

func (s *Service) buildTaskBinding(input *QueryInput) prompt.Task {
	task := input.Query
	return prompt.Task{Prompt: task, Attachments: input.Attachments}
}

// buildHistory derives history from a provided conversation transcript.
// It maps transcript turns and messages into prompt history without
// applying any overflow preview logic.
func (s *Service) buildHistory(ctx context.Context, transcript apiconv.Transcript) (prompt.History, error) {
	h, _, _, _, err := s.buildChronologicalHistory(ctx, transcript, nil, false)
	return h, err
}

// buildHistoryWithLimit maps transcript into prompt history applying overflow
// preview to user/assistant text messages and collecting current-turn
// elicitation messages separately.
func (s *Service) buildHistoryWithLimit(ctx context.Context, transcript apiconv.Transcript, input *QueryInput) (prompt.History, []*prompt.Message, bool, int, error) {
	// When no preview limit is configured, fall back to default mapping.
	if s.defaults == nil || s.defaults.PreviewSettings.Limit <= 0 {
		h, err := s.buildHistory(ctx, transcript)
		return h, nil, false, 0, err
	}
	h, elicitation, overflow, maxOverflowBytes, err := s.buildChronologicalHistory(ctx, transcript, input, true)
	return h, elicitation, overflow, maxOverflowBytes, err
}

// buildChronologicalHistory constructs prompt history turns from the provided
// transcript. When applyPreview is true, it applies overflow preview to
// user/assistant text messages using the service's effective preview limit.
// It also extracts current-turn elicitation messages as a separate slice.
func (s *Service) buildChronologicalHistory(
	ctx context.Context,
	transcript apiconv.Transcript,
	input *QueryInput,
	applyPreview bool,
) (prompt.History, []*prompt.Message, bool, int, error) {
	var out prompt.History
	var elicitation []*prompt.Message
	// Empty transcript yields empty history.
	if transcript == nil || len(transcript) == 0 {
		return out, nil, false, 0, nil
	}

	// Skip queued turns so future user prompts do not get merged into a single
	// LLM request when the chat queue is used. Always keep the current turn
	// (by TurnMeta) even if it is still marked queued due to eventual
	// consistency or ordering.
	currentTurnID := ""
	if tm, ok := memory.TurnMetaFromContext(ctx); ok {
		currentTurnID = strings.TrimSpace(tm.TurnID)
	}

	lastAssistantMessage := transcript.LastAssistantMessage()
	lastElicitationMessage := transcript.LastElicitationMessage()
	currentElicitation := false
	if lastElicitationMessage != nil && lastAssistantMessage != nil {
		if lastElicitationMessage.Id == lastAssistantMessage.Id {
			currentElicitation = true
		}
		if lastElicitationMessage.CreatedAt.After(lastAssistantMessage.CreatedAt) {
			currentElicitation = true
		}
	}

	type normalizedMsg struct {
		turnIdx int
		msg     *apiconv.Message
	}

	var normalized []normalizedMsg

	// Determine whether continuation preview format is enabled for the selected model.
	allowContinuation := s.allowContinuationPreview(ctx, input)

	// First pass: filter transcript messages according to existing rules,
	// and collect current-turn elicitation separately.
	for ti, turn := range transcript {
		if turn == nil || turn.Message == nil {
			continue
		}
		turnStatus := strings.ToLower(strings.TrimSpace(turn.Status))
		if turnStatus == "queued" && strings.TrimSpace(turn.Id) != currentTurnID {
			continue
		}
		// Exclude canceled turns from prompt history; canceled queue items should
		// not become part of model context for subsequent turns.
		if turnStatus == "canceled" && strings.TrimSpace(turn.Id) != currentTurnID {
			continue
		}
		messages := turn.GetMessages()
		for _, m := range messages {
			if m == nil {
				continue
			}
			// Allow error messages exactly once in the preview/limited
			// history path; include them even if type is not text,
			// provided they are not archived.
			if applyPreview && m.Status != nil && strings.EqualFold(strings.TrimSpace(*m.Status), "error") {
				if m.IsArchived() {
					continue
				}
				normalized = append(normalized, normalizedMsg{turnIdx: ti, msg: m})
				continue
			}
			if m.IsArchived() || m.IsInterim() {
				continue
			}
			// Skip canceled messages (queue deletions mark the user message as cancel).
			// Also include a defensive check for "canceled" which may appear on
			// assistant/tool messages via transcript hooks.
			if m.Status != nil {
				ms := strings.ToLower(strings.TrimSpace(*m.Status))
				if ms == "cancel" || ms == "canceled" {
					continue
				}
			}

			// Tool-call messages: always include when they have a
			// non-empty body (from response payload or content).
			if m.ToolCall != nil {
				if body := strings.TrimSpace(m.GetContent()); body != "" {
					normalized = append(normalized, normalizedMsg{turnIdx: ti, msg: m})
				}
				continue
			}

			// Attachment carrier messages are persisted as non-text control
			// messages and must still be considered for history so we can merge
			// their binaries into the correct parent prompt message.
			if isAttachmentCarrier(m) {
				normalized = append(normalized, normalizedMsg{turnIdx: ti, msg: m})
				continue
			}

			if m.Content == nil || *m.Content == "" {
				continue
			}
			mtype := strings.ToLower(strings.TrimSpace(m.Type))
			isElicitationType := mtype == "elicitation_request" || mtype == "elicitation_response"
			if mtype != "text" && !isElicitationType {
				continue
			}
			role := strings.ToLower(strings.TrimSpace(m.Role))

			if currentElicitation && lastElicitationMessage != nil && (role == "user" || role == "assistant") {
				if m.Id == lastElicitationMessage.Id && m.Content != nil {
					kind := prompt.MessageKindElicitAnswer
					if role == "assistant" {
						kind = prompt.MessageKindElicitPrompt
					}
					elicitation = append(elicitation, &prompt.Message{Kind: kind, Role: m.Role, Content: *m.Content, CreatedAt: m.CreatedAt})
					continue
				}

				if lastElicitationMessage.CreatedAt.Before(m.CreatedAt) && m.Content != nil {
					kind := prompt.MessageKindElicitAnswer
					if role == "assistant" {
						kind = prompt.MessageKindElicitPrompt
					}
					elicitation = append(elicitation, &prompt.Message{Kind: kind, Role: m.Role, Content: *m.Content, CreatedAt: m.CreatedAt})
					continue
				}
			}

			if role == "user" || role == "assistant" {
				normalized = append(normalized, normalizedMsg{turnIdx: ti, msg: m})
			}
		}
	}

	// Dedupe current-turn user task messages that are effectively the
	// same instruction expressed twice (e.g., raw input and a later
	// Task: wrapper). The transcript remains unchanged for UI/summary;
	// this is only a prompt-history optimization for the LLM.
	if len(normalized) > 0 {
		if tm, ok := memory.TurnMetaFromContext(ctx); ok && strings.TrimSpace(tm.TurnID) != "" {
			// Find the index of the current turn in the transcript
			currentTurnIdx := -1
			for ti, turn := range transcript {
				if turn == nil {
					continue
				}
				if strings.TrimSpace(turn.Id) == strings.TrimSpace(tm.TurnID) {
					currentTurnIdx = ti
					break
				}
			}
			if currentTurnIdx != -1 {
				// Collect indices of user messages in the current turn
				userIdxs := []int{}
				for i, item := range normalized {
					if item.turnIdx != currentTurnIdx || item.msg == nil {
						continue
					}
					role := strings.ToLower(strings.TrimSpace(item.msg.Role))
					if role != "user" {
						continue
					}
					userIdxs = append(userIdxs, i)
				}
				// If we have multiple user messages in the current turn
				// with the same normalized content (ignoring a leading
				// "Task:" wrapper), keep only the last one.
				if len(userIdxs) > 1 {
					last := userIdxs[len(userIdxs)-1]
					lastMsg := normalized[last].msg
					lastText := normalizeUserTaskContent(lastMsg.GetContent())
					if lastText != "" {
						filtered := make([]normalizedMsg, 0, len(normalized))
						for i, item := range normalized {
							// Drop earlier user messages in the current
							// turn that normalize to the same content.
							if i < last && item.turnIdx == currentTurnIdx && item.msg != nil {
								role := strings.ToLower(strings.TrimSpace(item.msg.Role))
								if role == "user" {
									if normalizeUserTaskContent(item.msg.GetContent()) == lastText {
										continue
									}
								}
							}
							filtered = append(filtered, item)
						}
						normalized = filtered
					}
				}
			}
		}
	}
	overflow := false
	maxOverflowBytes := 0
	turns := make([]*prompt.Turn, len(transcript))
	totalTurns := len(transcript)
	lastUserByTurn := map[string]*prompt.Message{}
	pendingUserAttachmentsByTurn := map[string][]*prompt.Attachment{}
	promptByMessageID := map[string]*prompt.Message{}
	pendingAttachmentsByMessageID := map[string][]*prompt.Attachment{}
	payloadAttachmentCache := map[string]*prompt.Attachment{}

	// Second pass: map normalized messages into prompt turns with optional preview.
	for _, item := range normalized {
		msg := item.msg
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		orig := ""
		if msg.ToolCall != nil {
			orig = strings.TrimSpace(msg.GetContent())
		} else if msg.Content != nil {
			orig = *msg.Content
		}
		text := orig
		if applyPreview && orig != "" {
			limit := s.turnPreviewLimit(item.turnIdx, totalTurns, true)
			if limit > 0 {
				preview, of := buildOverflowPreview(orig, limit, msg.Id, allowContinuation)
				if of {
					overflow = true
					if size := len(orig); size > maxOverflowBytes {
						maxOverflowBytes = size
					}
				}
				text = preview
			}
		}

		attachments, err := s.attachmentsFromMessage(ctx, msg, payloadAttachmentCache)
		if err != nil {
			return out, elicitation, overflow, maxOverflowBytes, err
		}

		turnIdx := item.turnIdx
		pt := turns[turnIdx]
		if pt == nil {
			pt = &prompt.Turn{ID: transcript[turnIdx].Id}
			turns[turnIdx] = pt
		}

		// Attachments are persisted as *control* child messages (QueryInput
		// attachments and tool-produced images). LLM providers need multimodal
		// content on user/system/assistant messages, so we merge carrier
		// attachments into the referenced parent message instead of emitting the
		// carrier itself.
		if isAttachmentCarrier(msg) && len(attachments) > 0 {
			parentID := ""
			if msg.ParentMessageId != nil {
				parentID = strings.TrimSpace(*msg.ParentMessageId)
			}
			if parentID != "" {
				if parent := promptByMessageID[parentID]; parent != nil {
					parent.Attachment = append(parent.Attachment, attachments...)
					debugAttachmentf("merged %d attachment(s) from carrier=%s into parent=%s", len(attachments), strings.TrimSpace(msg.Id), parentID)
				} else {
					pendingAttachmentsByMessageID[parentID] = append(pendingAttachmentsByMessageID[parentID], attachments...)
					debugAttachmentf("queued %d attachment(s) from carrier=%s for parent=%s", len(attachments), strings.TrimSpace(msg.Id), parentID)
				}
				continue
			}

			turnID := strings.TrimSpace(pt.ID)
			if turnID != "" {
				if last := lastUserByTurn[turnID]; last != nil {
					last.Attachment = append(last.Attachment, attachments...)
					debugAttachmentf("merged %d attachment(s) from carrier=%s into last user message=%s (turn=%s)", len(attachments), strings.TrimSpace(msg.Id), strings.TrimSpace(last.ID), turnID)
					continue
				}
				pendingUserAttachmentsByTurn[turnID] = append(pendingUserAttachmentsByTurn[turnID], attachments...)
				debugAttachmentf("queued %d attachment(s) from carrier=%s for turn=%s", len(attachments), strings.TrimSpace(msg.Id), turnID)
				continue
			}
			// Fallback: if turn id is missing, append to task-scoped attachments
			// so the binaries still reach the model with the user prompt.
			if input != nil {
				input.Attachments = append(input.Attachments, attachments...)
			}
			continue
		}

		pmsgRole := role
		// Normalize tool-call messages to assistant role for history so
		// they are rendered as assistant context rather than tool role
		// messages, which require a preceding tool_calls message.
		if msg.ToolCall != nil {
			pmsgRole = "assistant"
		}

		pmsg := &prompt.Message{
			Role:       pmsgRole,
			Content:    text,
			Attachment: attachments,
			CreatedAt:  msg.CreatedAt,
			ID:         msg.Id,
		}
		msgID := strings.TrimSpace(msg.Id)
		if msgID != "" {
			promptByMessageID[msgID] = pmsg
			if pending := pendingAttachmentsByMessageID[msgID]; len(pending) > 0 {
				pmsg.Attachment = append(pmsg.Attachment, pending...)
				delete(pendingAttachmentsByMessageID, msgID)
			}
		}

		// Classify message kind and, when applicable, attach tool metadata.
		if msg.ToolCall != nil {
			pmsg.Kind = prompt.MessageKindToolResult
			pmsg.ToolOpID = msg.ToolCall.OpId
			pmsg.ToolName = msg.ToolCall.ToolName
			pmsg.ToolArgs = msg.ToolCallArguments()
			if msg.ToolCall.TraceId != nil {
				pmsg.ToolTraceID = strings.TrimSpace(*msg.ToolCall.TraceId)
			}
		} else {
			// Classify chat and elicitation messages. For past elicitation
			// flows, ElicitationId will be set on assistant/user messages;
			// current-turn elicitation has already been extracted into the
			// separate elicitation slice and will not reach this block.
			if msg.ElicitationId != nil && strings.TrimSpace(*msg.ElicitationId) != "" {
				if role == "assistant" {
					pmsg.Kind = prompt.MessageKindElicitPrompt
				} else if role == "user" {
					pmsg.Kind = prompt.MessageKindElicitAnswer
				}
			} else {
				if role == "user" {
					pmsg.Kind = prompt.MessageKindChatUser
				} else if role == "assistant" {
					pmsg.Kind = prompt.MessageKindChatAssistant
				}
			}
		}
		pt.Messages = append(pt.Messages, pmsg)
		if pt.StartedAt.IsZero() || msg.CreatedAt.Before(pt.StartedAt) {
			pt.StartedAt = msg.CreatedAt
		}

		// Track the last user message per turn so deferred tool attachments
		// can be applied once a user message exists.
		if strings.EqualFold(strings.TrimSpace(pmsg.Role), "user") {
			turnID := strings.TrimSpace(pt.ID)
			if turnID != "" {
				lastUserByTurn[turnID] = pmsg
				if pending := pendingUserAttachmentsByTurn[turnID]; len(pending) > 0 {
					pmsg.Attachment = append(pmsg.Attachment, pending...)
					delete(pendingUserAttachmentsByTurn, turnID)
				}
			}
		}

		// Archive error messages once processed when applyPreview is enabled.
		if applyPreview && msg.Status != nil && strings.EqualFold(strings.TrimSpace(*msg.Status), "error") {
			if !msg.IsArchived() {
				if mm := msg.NewMutable(); mm != nil {
					archived := 1
					mm.Archived = &archived
					mm.Has.Archived = true
					if err := s.conversation.PatchMessage(ctx, (*apiconv.MutableMessage)(mm)); err != nil {
						return out, elicitation, overflow, maxOverflowBytes, fmt.Errorf("failed to archive error message %q: %w", msg.Id, err)
					}
				}
			}
		}
	}

	// If a turn had only control attachment messages (no parent message present
	// due to filtering), fall back to task-scoped attachments so they still
	// reach the model.
	if input != nil {
		for _, pending := range pendingUserAttachmentsByTurn {
			if len(pending) == 0 {
				continue
			}
			input.Attachments = append(input.Attachments, pending...)
		}
		for _, pending := range pendingAttachmentsByMessageID {
			if len(pending) == 0 {
				continue
			}
			input.Attachments = append(input.Attachments, pending...)
		}
	}

	// Finalize turns: drop nils, sort messages by CreatedAt, and build the
	// legacy flat Messages view for persisted history.
	for _, t := range turns {
		if t == nil || len(t.Messages) == 0 {
			continue
		}
		sort.SliceStable(t.Messages, func(i, j int) bool {
			return t.Messages[i].CreatedAt.Before(t.Messages[j].CreatedAt)
		})
		out.Past = append(out.Past, t)
		for _, m := range t.Messages {
			out.Messages = append(out.Messages, m)
		}
	}

	return out, elicitation, overflow, maxOverflowBytes, nil
}

func isAttachmentCarrier(msg *apiconv.Message) bool {
	if msg == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(msg.Type), "control") {
		return false
	}
	// Real tool op/result messages carry ToolCall; attachment carriers do not.
	if msg.ToolCall != nil {
		return false
	}
	if msg.AttachmentPayloadId != nil && strings.TrimSpace(*msg.AttachmentPayloadId) != "" {
		return true
	}
	return msg.Attachment != nil && len(msg.Attachment) > 0
}

func (s *Service) attachmentsFromMessage(ctx context.Context, msg *apiconv.Message, cache map[string]*prompt.Attachment) ([]*prompt.Attachment, error) {
	if msg == nil {
		return nil, nil
	}
	attachments := attachmentsFromMessageView(msg)

	if msg.AttachmentPayloadId == nil || strings.TrimSpace(*msg.AttachmentPayloadId) == "" {
		return attachments, nil
	}
	if s.conversation == nil {
		return nil, fmt.Errorf("conversation API not configured")
	}
	payloadID := strings.TrimSpace(*msg.AttachmentPayloadId)

	if cache != nil {
		if cached, ok := cache[payloadID]; ok && cached != nil {
			return append(attachments, cached), nil
		}
	}

	payload, err := s.conversation.GetPayload(ctx, payloadID)
	if err != nil {
		return nil, fmt.Errorf("get attachment payload %q: %w", payloadID, err)
	}
	if payload == nil {
		return nil, fmt.Errorf("get attachment payload %q: not found", payloadID)
	}
	var data []byte
	if payload.InlineBody != nil && len(*payload.InlineBody) > 0 {
		data = make([]byte, len(*payload.InlineBody))
		copy(data, *payload.InlineBody)
	} else if payload.URI != nil && strings.TrimSpace(*payload.URI) != "" {
		downloaded, err := afs.New().DownloadWithURL(ctx, strings.TrimSpace(*payload.URI))
		if err != nil {
			return nil, fmt.Errorf("download attachment payload uri %q: %w", strings.TrimSpace(*payload.URI), err)
		}
		data = downloaded
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("attachment payload %q has no data", payloadID)
	}

	name := ""
	if msg.Content != nil {
		name = strings.TrimSpace(*msg.Content)
	}
	if name == "" {
		name = "(attachment)"
	}
	uri := ""
	if payload.URI != nil {
		uri = strings.TrimSpace(*payload.URI)
	}
	mimeType := strings.TrimSpace(payload.MimeType)
	att := &prompt.Attachment{
		Name: name,
		URI:  uri,
		Mime: mimeType,
		Data: data,
	}
	debugAttachmentf("loaded attachment payload=%s bytes=%d mime=%s name=%s", payloadID, len(data), mimeType, name)
	if cache != nil {
		cache[payloadID] = att
	}
	attachments = append(attachments, att)
	return attachments, nil
}

func attachmentsFromMessageView(msg *apiconv.Message) []*prompt.Attachment {
	if msg == nil || msg.Attachment == nil || len(msg.Attachment) == 0 {
		return nil
	}
	defaultName := ""
	if msg.Content != nil {
		defaultName = strings.TrimSpace(*msg.Content)
	}
	var attachments []*prompt.Attachment
	for _, av := range msg.Attachment {
		if av == nil {
			continue
		}
		var data []byte
		if av.InlineBody != nil && len(*av.InlineBody) > 0 {
			data = append([]byte(nil), (*av.InlineBody)...)
		} else {
			// Skip attachment views that don't carry bytes. For prompt construction
			// we rely on attachment carrier messages (AttachmentPayloadId) to fetch
			// the binary payload, avoiding large blobs in transcript payloads.
			continue
		}
		uri := ""
		if av.Uri != nil {
			uri = strings.TrimSpace(*av.Uri)
		}
		name := defaultName
		if name == "" && uri != "" {
			name = path.Base(uri)
		}
		mimeType := strings.TrimSpace(av.MimeType)
		attachments = append(attachments, &prompt.Attachment{
			Name: name,
			URI:  uri,
			Mime: mimeType,
			Data: data,
		})
	}
	return attachments
}

// appendCurrentMessages appends messages to History.Current ensuring
// CreatedAt is set and non-decreasing within the current turn. It does
// not modify Past timestamps.
func appendCurrentMessages(h *prompt.History, msgs ...*prompt.Message) {
	if h == nil || len(msgs) == 0 {
		return
	}
	if h.Current == nil {
		h.Current = &prompt.Turn{ID: h.CurrentTurnID}
	}
	for _, m := range msgs {
		if m == nil {
			continue
		}
		// Seed CreatedAt when zero.
		if m.CreatedAt.IsZero() {
			m.CreatedAt = time.Now().UTC()
		}
		// Ensure non-decreasing CreatedAt relative to the last current message.
		if n := len(h.Current.Messages); n > 0 {
			last := h.Current.Messages[n-1].CreatedAt
			if m.CreatedAt.Before(last) {
				m.CreatedAt = last.Add(time.Nanosecond)
			}
		}
		h.Current.Messages = append(h.Current.Messages, m)
	}
}

// buildToolExecutions extracts tool calls from the provided conversation transcript for the current turn.
func (s *Service) buildToolExecutions(ctx context.Context, input *QueryInput, conv *apiconv.Conversation, exposure agent.ToolCallExposure) ([]*llm.ToolCall, bool, int, error) {
	turnMeta, ok := memory.TurnMetaFromContext(ctx)
	if !ok || strings.TrimSpace(turnMeta.TurnID) == "" {
		return nil, false, 0, nil
	}
	transcript := conv.GetTranscript()
	// Determine whether continuation preview format is enabled for the selected model.
	allowContinuation := s.allowContinuationPreview(ctx, input)
	totalTurns := len(transcript)
	overflowFound := false
	maxOverflowBytes := 0
	buildFromTurn := func(turnIdx int, t *apiconv.Turn, applyAging bool) []*llm.ToolCall {
		var out []*llm.ToolCall
		if t == nil {
			return out
		}

		toolCalls := t.ToolCalls()
		if len(toolCalls) > s.defaults.ToolCallMaxResults && s.defaults.ToolCallMaxResults > 0 {
			toolCalls = toolCalls[len(toolCalls)-s.defaults.ToolCallMaxResults:]
		}
		limit := s.turnPreviewLimit(turnIdx, totalTurns, applyAging)
		for _, m := range toolCalls {
			args := m.ToolCallArguments()
			// Prepare result content for LLM: derive preview from message content with per-turn limit
			result := ""
			if body := strings.TrimSpace(m.GetContent()); body != "" && limit > 0 {
				preview, overflow := buildOverflowPreview(body, limit, m.Id, allowContinuation)
				if overflow {
					overflowFound = true
					if size := len(body); size > maxOverflowBytes {
						maxOverflowBytes = size
					}
				}
				result = preview
			}

			// Canonicalize tool name so it matches declared tool signatures for providers.
			tc := llm.NewToolCall(m.ToolCall.OpId, mcpname.Canonical(m.ToolCall.ToolName), args, result)
			out = append(out, &tc)
		}
		return out
	}

	switch strings.ToLower(string(exposure)) {
	case "conversation":
		var out []*llm.ToolCall
		for idx, t := range transcript {
			out = append(out, buildFromTurn(idx, t, true)...)
		}
		return out, overflowFound, maxOverflowBytes, nil
	case "turn", "":
		// Find current turn only
		var aTurn *apiconv.Turn
		var turnIdx int
		for idx, t := range transcript {
			if t != nil && t.Id == turnMeta.TurnID {
				aTurn = t
				turnIdx = idx
				break
			}
		}
		if aTurn == nil {
			return nil, false, 0, nil
		}
		// For turn exposure, do not apply aging; always use Limit.
		execs := buildFromTurn(turnIdx, aTurn, false)
		return execs, overflowFound, maxOverflowBytes, nil
	default:
		// Unrecognised/semantic: do not include tool calls for now
		return nil, false, 0, nil
	}
}

// turnPreviewLimit returns the preview limit for a given turn index,
// applying aging only to older turns. The newest
// PreviewSettings.AgedAfterSteps turns use Limit, older ones (when
// AgedLimit > 0) use AgedLimit. Aging is never applied to the
// synthetic Current turn.
func (s *Service) turnPreviewLimit(turnIdx, totalTurns int, applyAging bool) int {
	if s.defaults == nil || s.defaults.PreviewSettings.Limit <= 0 {
		return 0
	}
	limit := s.defaults.PreviewSettings.Limit
	if !applyAging || s.defaults.PreviewSettings.AgedAfterSteps <= 0 || s.defaults.PreviewSettings.AgedLimit <= 0 {
		return limit
	}
	w := s.defaults.PreviewSettings.AgedAfterSteps
	if totalTurns <= w {
		return limit
	}
	// Turns with index < totalTurns-w are considered aged.
	if turnIdx < totalTurns-w {
		return s.defaults.PreviewSettings.AgedLimit
	}
	return limit
}

func (s *Service) buildToolSignatures(ctx context.Context, input *QueryInput) ([]*llm.ToolDefinition, bool, error) {
	if s.registry == nil || input == nil || input.Agent == nil {
		return nil, false, nil
	}
	tools, err := s.resolveTools(ctx, input)
	if err != nil {
		return nil, false, err
	}
	out := padapter.ToToolDefinitions(tools)
	return out, len(out) > 0, nil
}

func (s *Service) buildDocumentsBinding(ctx context.Context, input *QueryInput, isSystem bool) (prompt.Documents, error) {
	var docs prompt.Documents
	var knowledge []*agent.Knowledge
	if isSystem {
		knowledge = input.Agent.SystemKnowledge
	} else {
		knowledge = input.Agent.Knowledge
	}
	urls := make([]string, 0, len(knowledge))
	for _, k := range knowledge {
		if k != nil && strings.TrimSpace(k.URL) != "" {
			urls = append(urls, k.URL)
		}
	}

	matchedDocs, err := s.matchDocuments(ctx, input, knowledge)
	if err != nil {
		return docs, err
	}

	docs.Items = padapter.FromSchemaDocs(matchedDocs)
	return docs, nil
}

// trimStr ensures s is at most n runes, appending ellipsis when truncated.
func trimStr(s string, n int) string {
	s = strings.TrimSpace(s)
	if n <= 0 || len(s) <= n {
		return s
	}
	if n > 3 {
		return s[:n-3] + "..."
	}
	return s[:n]
}

// normalizeUserTaskContent trims whitespace and strips a leading
// "Task:" wrapper (case-insensitive, on the first line) so we can
// detect semantically equivalent user instructions such as a raw
// query and a later "Task:" form.
func normalizeUserTaskContent(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Fast path: no Task: prefix on first line.
	lower := strings.ToLower(s)
	if !strings.HasPrefix(lower, "task:") && !strings.Contains(lower, "\n") {
		return s
	}
	lines := strings.SplitN(s, "\n", 2)
	if len(lines) == 0 {
		return s
	}
	first := strings.TrimSpace(lines[0])
	if !strings.HasPrefix(strings.ToLower(first), "task:") {
		return s
	}
	if len(lines) == 1 {
		// Only "Task:" line present; treat as empty content.
		return ""
	}
	// Use the remainder as the normalized task content.
	return strings.TrimSpace(lines[1])
}
