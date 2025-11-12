package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/viant/afs/url"
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/llm"
	base "github.com/viant/agently/genai/llm/provider/base"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/prompt"
	padapter "github.com/viant/agently/genai/prompt/adapter"
	"github.com/viant/agently/genai/service/augmenter"
	mcpfs "github.com/viant/agently/genai/service/augmenter/mcpfs"
	"github.com/viant/agently/genai/service/core"
	mcpuri "github.com/viant/agently/internal/mcp/uri"
	"github.com/viant/agently/internal/workspace"
	mcpname "github.com/viant/agently/pkg/mcpname"
	embSchema "github.com/viant/embedius/schema"
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
	conv, err := s.fetchConversationWithRetry(ctx, input.ConversationID, apiconv.WithIncludeToolCall(true))
	if err != nil {
		return nil, err
	}
	if conv == nil {
		return nil, fmt.Errorf("conversation not found: %s", strings.TrimSpace(input.ConversationID))
	}

	// Compute effective preview limit using service defaults only
	hist, histOverflow, err := s.buildHistoryWithLimit(ctx, conv.GetTranscript(), &input.RequestTime)
	if err != nil {
		return nil, err
	}
	b.History = hist
	// Populate History.LastResponse using the last assistant message in transcript
	if conv != nil {
		tr := conv.GetTranscript()
		if last := tr.LastAssistantMessage(); last != nil {
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
	// Merge latest user elicitation payload (JSON object) into binding.Context
	mergeElicitationPayloadIntoContext(&b.History, &b.Context)
	if histOverflow {
		b.Flags.HasMessageOverflow = true
	}

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

	// Append internal tools needed for continuation flows (no duplicates)
	s.ensureInternalToolsIfNeeded(ctx, input, b)

	docs, err := s.buildDocumentsBinding(ctx, input, false)
	if err != nil {
		return nil, err
	}
	b.Documents = docs
	// Normalize user doc URIs by trimming workspace root for stable display
	s.normalizeDocURIs(&b.Documents, workspace.Root())
	// Attach non-text user documents as binary attachments (e.g., PDFs, images)
	s.attachNonTextUserDocs(ctx, b)

	b.SystemDocuments, err = s.buildDocumentsBinding(ctx, input, true)
	if err != nil {
		return nil, err
	}
	// Normalize system doc URIs similarly (even if not rendered now)
	s.normalizeDocURIs(&b.SystemDocuments, workspace.Root())
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
				result[ckey] = &prompt.Trace{ID: ckey, Kind: prompt.KindContent, At: m.CreatedAt}
			}
		}
	}
	return result
}

// mergeElicitationPayloadIntoContext folds the most recent JSON object payloads
// from user elicitation messages into the binding context so downstream plans
// can see resolved inputs (e.g., workdir). Later messages win on key collision.
func mergeElicitationPayloadIntoContext(h *prompt.History, ctxPtr *map[string]interface{}) {
	if h == nil || len(h.UserElicitation) == 0 {
		return
	}
	// Ensure context map exists
	if ctxPtr == nil {
		return
	}
	if *ctxPtr == nil {
		*ctxPtr = map[string]interface{}{}
	}
	ctx := *ctxPtr
	// Process in order, last one wins
	for _, m := range h.UserElicitation {
		if m == nil {
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
		for k, v := range payload {
			ctx[k] = v
		}
	}
}

// fetchConversationWithRetry attempts to fetch a conversation up to three times,
// applying a short exponential backoff on transient errors. It returns an error
// when the conversation is missing or on non-transient failures.
func (s *Service) fetchConversationWithRetry(ctx context.Context, id string, options ...apiconv.Option) (*apiconv.Conversation, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		conv, err := s.conversation.GetConversation(ctx, id, options...)
		if err == nil {
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
			if method == "show" || method == "summarize" || method == "match" {
				allowed = true
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
// continuing a prior response. Tools are appended only when the selected model
// supports continuation. Duplicates are avoided by canonical name.
func (s *Service) ensureInternalToolsIfNeeded(ctx context.Context, input *QueryInput, b *prompt.Binding) {
	if s == nil || s.registry == nil || b == nil {
		return
	}
	modelName := strings.TrimSpace(b.Model)
	if modelName == "" {
		return
	}

	// Approximate continuationEnabled: require provider support for continuation.
	if !s.llm.ModelImplements(ctx, modelName, base.SupportsContinuationByResponseID) {
		return
	}

	if input.Agent.SupportsContinuationByResponseID != nil && !*input.Agent.SupportsContinuationByResponseID {
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

// debugf prints binding-level debug logs when AGENTLY_DEBUG or AGENTLY_DEBUG_BINDING is set.
// debugf is intentionally a no-op to avoid noisy logs during normal operation.
// Keep the function for quick re‑enablement if needed while troubleshooting.

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

	// Auto/full decision for MCP: if MinScore set -> match; otherwise list resources
	// and if any location has more entries than MaxFiles (default 5) -> match; else full.
	useMatch := false
	if cfg.MinScore != nil {
		useMatch = true
	}
	if !useMatch {
		// Count resources per location using mcpfs
		fs := mcpfs.New(s.mcpMgr)
		for _, loc := range locations {
			objects, err := fs.List(ctx, loc)
			if err != nil {
				// On error, fall back to match path
				useMatch = true
				break
			}
			limit := cfg.MaxFiles
			if limit <= 0 && s.defaults != nil && s.defaults.Match.MaxFiles > 0 {
				limit = s.defaults.Match.MaxFiles
			}
			if len(objects) > max(1, limit) {
				useMatch = true
				break
			}
		}
	}
	if !useMatch {
		// Full mode: download each resource and attach directly
		fs := mcpfs.New(s.mcpMgr)
		var out []*prompt.Document
		for _, loc := range locations {
			objects, err := fs.List(ctx, loc)
			if err != nil {
				continue
			}
			// Stable order by normalized URL for caching
			sort.SliceStable(objects, func(i, j int) bool {
				ui := strings.ToLower(strings.TrimSpace(objects[i].URL()))
				uj := strings.ToLower(strings.TrimSpace(objects[j].URL()))
				return ui < uj
			})
			// Cap by MaxFiles if specified
			limit := cfg.MaxFiles
			if limit <= 0 && s.defaults != nil && s.defaults.Match.MaxFiles > 0 {
				limit = s.defaults.Match.MaxFiles
			}
			limit = max(1, limit)
			n := len(objects)
			if limit > 0 && n > limit {
				n = limit
			}
			for i := 0; i < n; i++ {
				o := objects[i]
				data, err := fs.Download(ctx, o)
				if err != nil || len(data) == 0 {
					continue
				}
				uri := o.URL()
				if strings.TrimSpace(cfg.TrimPath) != "" {
					uri = strings.TrimPrefix(uri, cfg.TrimPath)
				}
				content := strings.TrimSpace(string(data))
				out = append(out, &prompt.Document{Title: baseName(uri), PageContent: content, SourceURI: uri})
			}
		}
		return out, nil
	}

	// Effective docs cap (entry or defaults)
	effMax := cfg.MaxFiles
	if effMax <= 0 && s.defaults != nil && s.defaults.Match.MaxFiles > 0 {
		effMax = s.defaults.Match.MaxFiles
	}
	augIn := &augmenter.AugmentDocsInput{
		Query:        input.Query,
		Locations:    locations,
		Match:        cfg.Match,
		Model:        input.EmbeddingModel,
		MaxDocuments: max(1, effMax),
		TrimPath:     cfg.TrimPath,
	}
	// Debug inputs
	inc := []string{}
	exc := []string{}
	if cfg.Match != nil {
		inc = append(inc, cfg.Match.Inclusions...)
		exc = append(exc, cfg.Match.Exclusions...)
	}
	augOut := &augmenter.AugmentDocsOutput{}
	if err := s.augmenter.AugmentDocs(ctx, augIn, augOut); err != nil {
		return nil, fmt.Errorf("failed to build MCP documents: %w", err)
	}
	// Optional minScore filter (keep order)
	if cfg.MinScore != nil {
		filtered := make([]embSchema.Document, 0, len(augOut.Documents))
		threshold := float32(*cfg.MinScore)
		for _, d := range augOut.Documents {
			if d.Score >= threshold {
				filtered = append(filtered, d)
			}
		}
		augOut.Documents = filtered
	}
	// Stable order by normalized source URI
	sort.SliceStable(augOut.Documents, func(i, j int) bool {
		si := strings.ToLower(strings.TrimSpace(extractSource(augOut.Documents[i].Metadata)))
		sj := strings.ToLower(strings.TrimSpace(extractSource(augOut.Documents[j].Metadata)))
		return si < sj
	})

	// Build binding documents from selected resources; fetch full content from MCP/FS.
	var docsOut []*prompt.Document
	unique := map[string]bool{}
	for i, d := range augOut.Documents {
		// Apply effective cap
		if effMax > 0 && i >= effMax {
			break
		}
		uri := extractSource(d.Metadata)
		if strings.TrimSpace(uri) == "" {
			continue
		}
		if strings.TrimSpace(cfg.TrimPath) != "" {
			uri = strings.TrimPrefix(uri, cfg.TrimPath)
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
		if mcpuri.Is(uri) && s.mcpMgr != nil {
			if resolved, err := s.fetchMCPResource(ctx, uri); err == nil && len(resolved) > 0 {
				data = resolved
			}
		} else {
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
func (s *Service) buildHistoryWithLimit(ctx context.Context, transcript apiconv.Transcript, requestTime *time.Time) (prompt.History, bool, error) {
	// When effectiveLimit <= 0, fall back to default
	var out prompt.History

	if s.effectivePreviewLimit(0) <= 0 {
		h, err := s.buildHistory(ctx, transcript)
		return h, false, err
	}
	anchor := transcript.LastAssistantMessage()
	normalized := transcript.Filter(func(v *apiconv.Message) bool {
		if v == nil || v.Content == nil || *v.Content == "" {
			return false
		}
		// Allow error messages exactly once
		if v.Status != nil && strings.EqualFold(strings.TrimSpace(*v.Status), "error") {
			if v.IsArchived() {
				return false
			}
			return true
		}
		if v.IsArchived() || v.IsInterim() {
			return false
		}
		if strings.ToLower(strings.TrimSpace(v.Type)) != "text" {
			return false
		}
		role := strings.ToLower(strings.TrimSpace(v.Role))

		if role == "user" || role == "assistant" {
			// Gate user/assistant messages strictly by anchorFrom when available.
			if anchor != nil && anchor.CreatedAt.Before(v.CreatedAt) && v.Content != nil {
				out.UserElicitation = append(out.UserElicitation, &prompt.Message{Role: v.Role, Content: *v.Content})
				return false
			}
			return true
		}
		return false
	})

	overflow := false
	for i, msg := range normalized {
		role := msg.Role
		content := ""
		if msg.Content != nil {
			// Apply overflow preview with message id reference
			preview, of := buildOverflowPreview(*msg.Content, s.effectivePreviewLimit(i), msg.Id)
			if of {
				overflow = true
			}
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

		if msg.Status != nil && strings.EqualFold(strings.TrimSpace(*msg.Status), "error") {
			if !msg.IsArchived() {
				if mm := msg.NewMutable(); mm != nil {
					archived := 1
					mm.Archived = &archived
					mm.Has.Archived = true
					err := s.conversation.PatchMessage(ctx, (*apiconv.MutableMessage)(mm))
					if err != nil {
						return out, overflow, fmt.Errorf("failed to archive error message %q: %w", msg.Id, err)
					}
				}
			}
		}

	}
	return out, overflow, nil
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

		toolCalls := t.ToolCalls()
		if len(toolCalls) > s.defaults.ToolCallMaxResults && s.defaults.ToolCallMaxResults > 0 {
			toolCalls = toolCalls[len(toolCalls)-s.defaults.ToolCallMaxResults:]
		}
		for i, m := range toolCalls {
			args := m.ToolCallArguments()

			effectivePreviewLimit := s.effectivePreviewLimit(i)

			// Prepare result content for LLM: derive preview from message content with effective limit
			result := ""
			if body := strings.TrimSpace(m.GetContent()); body != "" {
				preview, overflow := buildOverflowPreview(body, effectivePreviewLimit, m.Id)
				if overflow {
					overflowFound = true
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

func (s *Service) effectivePreviewLimit(step int) int {

	if s.defaults.PreviewSettings.AgedAfterSteps > 0 && step > s.defaults.PreviewSettings.AgedAfterSteps && s.defaults.PreviewSettings.AgedLimit > 0 {
		return s.defaults.PreviewSettings.AgedLimit
	}

	effectiveCallToolResultLimit := 0
	// Use service defaults only
	if s.defaults != nil && s.defaults.PreviewSettings.Limit > 0 {
		effectiveCallToolResultLimit = s.defaults.PreviewSettings.Limit
	}
	return effectiveCallToolResultLimit
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
