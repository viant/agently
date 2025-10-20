package agent

import (
	"context"
	"encoding/base64"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/viant/afs/url"
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/llm"
	base "github.com/viant/agently/genai/llm/provider/base"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/prompt"
	padapter "github.com/viant/agently/genai/prompt/adapter"
	"github.com/viant/agently/genai/service/augmenter"
	mcpuri "github.com/viant/agently/internal/mcp/uri"
	mcpname "github.com/viant/agently/pkg/mcpname"
	mcpschema "github.com/viant/mcp-protocol/schema"
)

func (s *Service) BuildBinding(ctx context.Context, input *QueryInput) (*prompt.Binding, error) {
	b := &prompt.Binding{}
	if input.Agent != nil {
		b.Model = input.Agent.Model
	}
	if strings.TrimSpace(input.ModelOverride) != "" {
		b.Model = strings.TrimSpace(input.ModelOverride)
	}
	// Fetch conversation transcript once and reuse; bubble up errors
	if s.conversation == nil {
		return nil, fmt.Errorf("conversation API not configured")
	}
	conv, err := s.conversation.GetConversation(ctx, input.ConversationID, apiconv.WithIncludeToolCall(true))
	if err != nil {
		return nil, err
	}

	// Compute effective preview limit (model override → model config → defaults)
	modelName := ""
	if input.Agent != nil {
		modelName = input.Agent.Model
	}
	if strings.TrimSpace(input.ModelOverride) != "" {
		modelName = strings.TrimSpace(input.ModelOverride)
	}
	effectiveLimit := 0
	if s.llm != nil {
		if v := s.llm.ModelToolPreviewLimit(modelName); v > 0 {
			effectiveLimit = v
		}
	}
	if effectiveLimit == 0 && s.defaults != nil && s.defaults.ToolCallResult.PreviewLimit > 0 {
		effectiveLimit = s.defaults.ToolCallResult.PreviewLimit
	}

	// Build history with overflow previews when limit is present
	hist, histOverflow, err := func() (prompt.History, bool, error) {
		// local wrapper to preserve current signature, then refactor
		// compute with clamping and track overflow occurrence
		if effectiveLimit <= 0 {
			h, e := s.buildHistory(ctx, conv.GetTranscript())
			return h, false, e
		}
		tr := conv.GetTranscript()
		normalized := tr.Filter(func(v *apiconv.Message) bool {
			if v == nil || v.IsArchived() || v.IsInterim() || v.Content == nil || *v.Content == "" {
				return false
			}
			if strings.ToLower(strings.TrimSpace(v.Type)) != "text" {
				return false
			}
			r := strings.ToLower(strings.TrimSpace(v.Role))
			return r == "user" || r == "assistant"
		})
		var out prompt.History
		overflow := false
		for _, msg := range normalized {
			role := msg.Role
			content := ""
			if msg.Content != nil {
				preview, of := buildOverflowPreview(*msg.Content, effectiveLimit, msg.Id)
				if of {
					overflow = true
				}
				content = preview
			}
			var attachments []*prompt.Attachment
			if msg.Attachment != nil && len(msg.Attachment) > 0 {
				for _, av := range msg.Attachment {
					if av == nil {
						continue
					}
					var data []byte
					if av.InlineBody != nil {
						data = []byte(*av.InlineBody)
					}
					name := ""
					if av.Uri != nil && *av.Uri != "" {
						name = path.Base(*av.Uri)
					}
					attachments = append(attachments, &prompt.Attachment{Name: name, URI: func() string {
						if av.Uri != nil {
							return *av.Uri
						}
						return ""
					}(), Mime: av.MimeType, Data: data})
				}
			}
			out.Messages = append(out.Messages, &prompt.Message{Role: role, Content: content, Attachment: attachments})
		}
		return out, overflow, nil
	}()
	if err != nil {
		return nil, err
	}
	b.History = hist
	if histOverflow {
		b.Flags.HasMessageOverflow = true
	}
	debugf("overflow flags: histOverflow=%v HasMessageOverflow=%v", histOverflow, b.Flags.HasMessageOverflow)
	b.Task = s.buildTaskBinding(input)

	b.Tools.Signatures, _, err = s.buildToolSignatures(ctx, input)
	if err != nil {
		return nil, err
	}
	// Tool executions exposure: default "turn"; allow QueryInput override; then agent setting.
	exposure := agent.ToolCallExposure("turn")
	if input.ToolCallExposure != nil && strings.TrimSpace(string(*input.ToolCallExposure)) != "" {
		exposure = *input.ToolCallExposure
	} else if input.Agent != nil && strings.TrimSpace(string(input.Agent.ToolCallExposure)) != "" {
		exposure = input.Agent.ToolCallExposure
	}
	execs, overflow, err := s.buildToolExecutions(ctx, input, conv, exposure)
	if err != nil {
		return nil, err
	}
	if len(execs) > 0 {
		b.Tools.Executions = execs
	}
	// Drive overflow-based helper exposure via binding flag
	if overflow {
		b.Flags.HasMessageOverflow = true
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

	docs, err := s.buildDocumentsBinding(ctx, input, false)
	if err != nil {
		return nil, err
	}
	b.Documents = docs

	b.SystemDocuments, err = s.buildDocumentsBinding(ctx, input, true)
	if err != nil {
		return nil, err
	}
	b.Context = input.Context

	// Optionally add MCP/resource-based documents selected via Embedius.
	// Previously these were attached as binary attachments; we now expose them
	// as binding documents so templates can reason over their content directly.
	if input.Agent != nil && input.Agent.MCPResources != nil && input.Agent.MCPResources.Enabled {
		if md, err := s.buildMCPDocuments(ctx, input, input.Agent.MCPResources); err != nil {
			return nil, err
		} else if len(md) > 0 {
			b.Documents.Items = append(b.Documents.Items, md...)
		}
	}
	return b, nil
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
					if strings.Contains(msg, "context limit exceeded") || strings.Contains(msg, "input exceeds the context window") {
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
	debugf("handleOverflow: tokenLimit=%v HasMessageOverflow=%v", tokenLimit, b.Flags.HasMessageOverflow)
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
	debugf("handleOverflow: matched %d internal/message tools", len(defs))
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
			if method == "show" || method == "summarize" || method == "match" {
				allowed = true
			}
		}
		debugf("handleOverflow: tool=%s method=%s allowed=%v already=%v", name, method, allowed, have[name])
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
	if s.defaults != nil && strings.TrimSpace(s.defaults.ToolCallResult.SystemGuidePath) != "" {
		guide := strings.TrimSpace(s.defaults.ToolCallResult.SystemGuidePath)
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

// debugf prints binding-level debug logs when AGENTLY_DEBUG or AGENTLY_DEBUG_BINDING is set.
// debugf is intentionally a no-op to avoid noisy logs during normal operation.
// Keep the function for quick re‑enablement if needed while troubleshooting.
func debugf(format string, args ...interface{}) {}

// buildMCPDocuments matches resources using Embedius and converts top-N results
// into binding documents. Indexing is handled lazily by the augmenter service.
func (s *Service) buildMCPDocuments(ctx context.Context, input *QueryInput, cfg *agent.MCPResources) ([]*prompt.Document, error) {
	if input == nil || cfg == nil {
		return nil, nil
	}
	// Require explicit embedding model and at least one location
	if strings.TrimSpace(input.EmbeddingModel) == "" {
		return nil, nil
	}
	locations := cfg.Locations
	if len(locations) == 0 {
		// No explicit locations provided; fall back to agent knowledge URLs
		for _, kn := range input.Agent.Knowledge {
			if kn != nil && strings.TrimSpace(kn.URL) != "" {
				locations = append(locations, kn.URL)
			}
		}
	}
	if len(locations) == 0 {
		return nil, nil
	}

	augIn := &augmenter.AugmentDocsInput{
		Query:        input.Query,
		Locations:    locations,
		Match:        cfg.Match,
		Model:        input.EmbeddingModel,
		MaxDocuments: max(1, cfg.MaxFiles),
		TrimPath:     cfg.TrimPath,
	}
	augOut := &augmenter.AugmentDocsOutput{}
	if err := s.augmenter.AugmentDocs(ctx, augIn, augOut); err != nil {
		return nil, fmt.Errorf("failed to build MCP documents: %w", err)
	}

	// Build binding documents from selected resources; fetch full content from MCP/FS.
	var docsOut []*prompt.Document
	unique := map[string]bool{}
	for i, d := range augOut.Documents {
		if cfg.MaxFiles > 0 && i >= cfg.MaxFiles {
			break
		}
		uri := extractSource(d.Metadata)
		if strings.TrimSpace(uri) == "" {
			continue
		}
		if unique[uri] {
			continue
		}
		unique[uri] = true

		// Fetch content
		var content []byte
		if mcpuri.Is(uri) && s.mcpMgr != nil {
			if resolved, err := s.fetchMCPResource(ctx, uri); err == nil && len(resolved) > 0 {
				content = resolved
			}
		}
		if len(content) == 0 && !mcpuri.Is(uri) {
			if raw, err := s.fs.DownloadWithURL(ctx, uri); err == nil && len(raw) > 0 {
				content = raw
			}
		}
		if len(content) == 0 && strings.TrimSpace(d.PageContent) != "" {
			content = []byte(d.PageContent)
		}

		docsOut = append(docsOut, &prompt.Document{
			Title:       baseName(uri),
			PageContent: string(content),
			SourceURI:   uri,
		})
	}
	return docsOut, nil
}

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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// fetchMCPResource resolves an mcp resource URI via MCP client.
func (s *Service) fetchMCPResource(ctx context.Context, source string) ([]byte, error) {
	turn, ok := memory.TurnMetaFromContext(ctx)
	if !ok || strings.TrimSpace(turn.ConversationID) == "" || s.mcpMgr == nil {
		return nil, fmt.Errorf("missing conversation or mcp manager")
	}
	server, resURI := parseMCPSource(source)
	if strings.TrimSpace(server) == "" || strings.TrimSpace(resURI) == "" {
		return nil, fmt.Errorf("invalid mcp source: %s", source)
	}
	cli, err := s.mcpMgr.Get(ctx, turn.ConversationID, server)
	if err != nil {
		return nil, fmt.Errorf("mcp get: %w", err)
	}
	if _, err := cli.Initialize(ctx); err != nil {
		return nil, fmt.Errorf("mcp init: %w", err)
	}
	res, err := cli.ReadResource(ctx, &mcpschema.ReadResourceRequestParams{Uri: resURI})
	if err != nil {
		return nil, fmt.Errorf("mcp read: %w", err)
	}
	var out []byte
	for _, c := range res.Contents {
		if c.Text != "" {
			out = append(out, []byte(c.Text)...)
			continue
		}
		if c.Blob != "" {
			if dec, err := base64.StdEncoding.DecodeString(c.Blob); err == nil {
				out = append(out, dec...)
			}
		}
	}
	return out, nil
}

// parseMCPSource supports mcp://server/path and mcp:server:/path formats.
func parseMCPSource(src string) (server, uri string) { return mcpuri.Parse(src) }

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

// buildHistory derives history from a provided conversation (if non-nil),
// otherwise falls back to DAO transcript for compatibility.
func (s *Service) buildHistory(ctx context.Context, transcript apiconv.Transcript) (prompt.History, error) {
	// Default behavior: no clamping; use transcript mapping.
	var h prompt.History
	h.Messages = transcript.History(false)
	return h, nil
}

// buildHistoryWithLimit maps transcript into prompt history applying overflow preview to user/assistant text messages.
func (s *Service) buildHistoryWithLimit(ctx context.Context, transcript apiconv.Transcript, limit int) (prompt.History, error) {
	// When limit <= 0, fall back to default
	if limit <= 0 {
		return s.buildHistory(ctx, transcript)
	}
	// Re-implement transcript.History(false) to include clamping with message id trailers.
	normalized := transcript.Filter(func(v *apiconv.Message) bool {
		if v == nil || v.IsArchived() || v.IsInterim() || v.Content == nil || *v.Content == "" {
			return false
		}
		if strings.ToLower(strings.TrimSpace(v.Type)) != "text" {
			return false
		}
		role := strings.ToLower(strings.TrimSpace(v.Role))
		return role == "user" || role == "assistant"
	})
	var out prompt.History
	for _, msg := range normalized {
		role := msg.Role
		content := ""
		if msg.Content != nil {
			// Apply overflow preview with message id reference
			preview, _ := buildOverflowPreview(*msg.Content, limit, msg.Id)
			content = preview
		}
		// Preserve attachments as in transcript.History
		var attachments []*prompt.Attachment
		if msg.Attachment != nil && len(msg.Attachment) > 0 {
			for _, av := range msg.Attachment {
				if av == nil {
					continue
				}
				var data []byte
				if av.InlineBody != nil {
					data = []byte(*av.InlineBody)
				}
				name := ""
				if av.Uri != nil && *av.Uri != "" {
					name = path.Base(*av.Uri)
				}
				attachments = append(attachments, &prompt.Attachment{Name: name, URI: func() string {
					if av.Uri != nil {
						return *av.Uri
					}
					return ""
				}(), Mime: av.MimeType, Data: data})
			}
		}
		out.Messages = append(out.Messages, &prompt.Message{Role: role, Content: content, Attachment: attachments})
	}
	return out, nil
}

// buildToolExecutions extracts tool calls from the provided conversation transcript for the current turn.
func (s *Service) buildToolExecutions(ctx context.Context, input *QueryInput, conv *apiconv.Conversation, exposure agent.ToolCallExposure) ([]*llm.ToolCall, bool, error) {
	turnMeta, ok := memory.TurnMetaFromContext(ctx)
	if !ok || strings.TrimSpace(turnMeta.TurnID) == "" {
		return nil, false, nil
	}
	transcript := conv.GetTranscript()
	overflowFound := false
	buildFromTurn := func(t *apiconv.Turn) []*llm.ToolCall {
		var out []*llm.ToolCall
		if t == nil {
			return out
		}
		// Determine effective preview limit: model-level override > service default
		modelName := ""
		if input.Agent != nil {
			modelName = input.Agent.Model
		}
		if strings.TrimSpace(input.ModelOverride) != "" {
			modelName = strings.TrimSpace(input.ModelOverride)
		}
		effectiveCallToolResultLimit := 0
		if s.llm != nil {
			if v := s.llm.ModelToolPreviewLimit(modelName); v > 0 {
				effectiveCallToolResultLimit = v
			}
		}
		if effectiveCallToolResultLimit == 0 && s.defaults != nil && s.defaults.ToolCallResult.PreviewLimit > 0 {
			effectiveCallToolResultLimit = s.defaults.ToolCallResult.PreviewLimit
		}
		for _, m := range t.ToolCalls() {
			args := m.ToolCallArguments()

			// Prepare result content for LLM: derive preview from message content with effective limit
			result := ""
			if body := strings.TrimSpace(m.GetContent()); body != "" {
				preview, overflow := buildOverflowPreview(body, effectiveCallToolResultLimit, m.Id)
				if overflow {
					overflowFound = true
				}
				// Mark overflow on the in-memory view so handleOverflow can auto-expose tools
				m.ToolCall.ResponseOverflow = overflow
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
		for _, t := range transcript {
			out = append(out, buildFromTurn(t)...)
		}
		return out, overflowFound, nil
	case "turn", "":
		// Find current turn only
		var aTurn *apiconv.Turn
		for _, t := range transcript {
			if t != nil && t.Id == turnMeta.TurnID {
				aTurn = t
				break
			}
		}
		if aTurn == nil {
			return nil, false, nil
		}
		execs := buildFromTurn(aTurn)
		return execs, overflowFound, nil
	default:
		// Unrecognised/semantic: do not include tool calls for now
		return nil, false, nil
	}
}

func (s *Service) buildToolSignatures(ctx context.Context, input *QueryInput) ([]*llm.ToolDefinition, bool, error) {
	if s.registry == nil || input.Agent == nil || len(input.Agent.Tool) == 0 {
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
