package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path"
	"strings"

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
	// Fetch conversation transcript once and reuse; bubble up errors
	if s.conversation == nil {
		return nil, fmt.Errorf("conversation API not configured")
	}
	conv, err := s.conversation.GetConversation(ctx, input.ConversationID, apiconv.WithIncludeToolCall(true))
	if err != nil {
		return nil, err
	}

	err = s.BuildHistory(ctx, conv.GetTranscript(), b)
	if err != nil {
		return nil, err
	}

	b.Task = s.buildTaskBinding(input)

	sig, _, err := s.buildToolSignatures(ctx, input)
	if err != nil {
		return nil, err
	}

	if len(sig) > 0 {
		b.Tools.Signatures = sig
		// Determine native tool-use capability from the selected model.
		model := ""
		if input.Agent != nil {
			model = input.Agent.Model
		}
		if strings.TrimSpace(input.ModelOverride) != "" {
			model = strings.TrimSpace(input.ModelOverride)
		}
		b.Flags.CanUseTool = s.llm != nil && s.llm.ModelImplements(ctx, model, base.CanUseTools)
	}
	// Tool executions exposure: default "turn"; allow QueryInput override; then agent setting.
	exposure := agent.ToolCallExposure("turn")
	if input.ToolCallExposure != nil && strings.TrimSpace(string(*input.ToolCallExposure)) != "" {
		exposure = *input.ToolCallExposure
	} else if input.Agent != nil && strings.TrimSpace(string(input.Agent.ToolCallExposure)) != "" {
		exposure = input.Agent.ToolCallExposure
	}
	if execs, err := s.buildToolExecutions(ctx, input, conv, exposure); err != nil {
		return nil, err
	} else if len(execs) > 0 {
		b.Tools.Executions = execs
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
	var h prompt.History
	h.Messages = transcript.History(false)
	return h, nil
}

// buildToolExecutions extracts tool calls from the provided conversation transcript for the current turn.
func (s *Service) buildToolExecutions(ctx context.Context, input *QueryInput, conv *apiconv.Conversation, exposure agent.ToolCallExposure) ([]*llm.ToolCall, error) {
	turnMeta, ok := memory.TurnMetaFromContext(ctx)
	if !ok || strings.TrimSpace(turnMeta.TurnID) == "" {
		return nil, nil
	}
	transcript := conv.GetTranscript()
	buildFromTurn := func(t *apiconv.Turn) []*llm.ToolCall {
		var out []*llm.ToolCall
		if t == nil {
			return out
		}
		for _, m := range t.ToolCalls() {
			args := map[string]interface{}{}
			// Prefer request payload (inline body JSON) for arguments
			if m.ToolCall.RequestPayload != nil && m.ToolCall.RequestPayload.InlineBody != nil {
				raw := strings.TrimSpace(*m.ToolCall.RequestPayload.InlineBody)
				if raw != "" {
					var parsed map[string]interface{}
					if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
						args = parsed
					}
				}
			}
			result := ""
			if m.ToolCall.ResponsePayload != nil && m.ToolCall.ResponsePayload.InlineBody != nil {
				result = strings.TrimSpace(*m.ToolCall.ResponsePayload.InlineBody)
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
		return out, nil
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
			return nil, nil
		}
		return buildFromTurn(aTurn), nil
	default:
		// Unrecognised/semantic: do not include tool calls for now
		return nil, nil
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
