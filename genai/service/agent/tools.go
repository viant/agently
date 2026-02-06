package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/viant/agently/genai/llm"
	toolbundle "github.com/viant/agently/genai/tool/bundle"
	mcpname "github.com/viant/agently/pkg/mcpname"
)

// Small utilities for tool pattern resolution and filtering.

// toolPatterns extracts tool selection patterns from the agent configuration.
// Shared across ensureTools and binding to avoid duplication.
func toolPatterns(qi *QueryInput) []string {
	var out []string
	if qi == nil || qi.Agent == nil {
		return out
	}
	for _, aTool := range qi.Agent.Tool.Items {
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
		out = append(out, pattern)
	}
	return out
}

// resolveTools resolves tools using the following precedence:
//   - If input.ToolsAllowed is provided and non-empty, resolve exactly those tools by name
//     and do not gate by agent patterns (explicit allow-list).
//   - Otherwise, resolve tools from agent patterns.
func (s *Service) resolveTools(ctx context.Context, qi *QueryInput) ([]llm.Tool, error) {
	// Clear any previous registry warnings before this resolution cycle.
	if w, ok := s.registry.(interface{ ClearWarnings() }); ok {
		w.ClearWarnings()
	}
	// Prefer explicit allow-list when provided and non-empty.

	if len(qi.ToolsAllowed) > 0 {
		var out []llm.Tool
		for _, n := range qi.ToolsAllowed {
			name := strings.TrimSpace(n)
			if name == "" {
				continue
			}
			if def, ok := s.registry.GetDefinition(name); ok && def != nil {
				out = append(out, llm.Tool{Type: "function", Definition: *def})
				continue
			}
			// Allowed tool not found: add a warning to query output via context.
			appendWarning(ctx, fmt.Sprintf("allowed tool not found: %s", name))
		}
		// Append any registry warnings (e.g., unreachable servers) to output warnings via context.
		if w, ok := s.registry.(interface {
			LastWarnings() []string
			ClearWarnings()
		}); ok {
			for _, msg := range w.LastWarnings() {
				appendWarning(ctx, msg)
			}
			w.ClearWarnings()
		}
		return out, nil
	}

	// Bundle selection: runtime override, then agent config.
	bundleIDs := selectedBundleIDs(qi)

	if len(bundleIDs) > 0 {
		defs, err := s.resolveBundleDefinitions(ctx, bundleIDs)
		if err != nil {
			return nil, err
		}
		// Allow agent tool patterns to further extend selection when present.
		extra := toolPatterns(qi)
		if len(extra) > 0 {
			for _, p := range extra {
				for _, def := range s.registry.MatchDefinition(p) {
					if def == nil {
						continue
					}
					defs = append(defs, *def)
				}
			}
		}
		defs = dedupeDefinitions(defs)
		tools := make([]llm.Tool, 0, len(defs))
		for i := range defs {
			tools = append(tools, llm.Tool{Type: "function", Definition: defs[i]})
		}
		tools = s.appendRegistryWarnings(ctx, tools)
		return tools, nil
	}

	// Fall back to agent patterns when no explicit allow-list is provided.
	patterns := toolPatterns(qi)
	if len(patterns) == 0 {
		return nil, nil
	}
	var out []llm.Tool
	for _, p := range patterns {
		for _, def := range s.registry.MatchDefinition(p) {
			out = append(out, llm.Tool{Type: "function", Definition: *def})
		}
	}
	// Append any registry warnings raised during matching.
	out = s.appendRegistryWarnings(ctx, out)
	return out, nil
}

func agentName(qi *QueryInput) string {
	if qi == nil || qi.Agent == nil {
		return ""
	}
	if strings.TrimSpace(qi.Agent.ID) != "" {
		return strings.TrimSpace(qi.Agent.ID)
	}
	return strings.TrimSpace(qi.Agent.Name)
}

func toolNames(in []llm.Tool) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for i := range in {
		if n := strings.TrimSpace(in[i].Definition.Name); n != "" {
			out = append(out, n)
		}
	}
	return out
}

func selectedBundleIDs(qi *QueryInput) []string {
	if qi == nil {
		return nil
	}
	// runtime override
	if len(qi.ToolBundles) > 0 {
		return normalizeStringList(qi.ToolBundles)
	}
	if qi.Agent == nil {
		return nil
	}
	return normalizeStringList(qi.Agent.Tool.Bundles)
}

func normalizeStringList(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	for _, raw := range in {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		key := strings.ToLower(id)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, id)
	}
	return out
}

func (s *Service) resolveBundleDefinitions(ctx context.Context, bundleIDs []string) ([]llm.ToolDefinition, error) {
	if s == nil || s.registry == nil {
		return nil, nil
	}
	bundles, err := s.loadBundles(ctx)
	if err != nil {
		return nil, err
	}
	var derived map[string]*toolbundle.Bundle
	if len(bundles) == 0 {
		derived = indexBundlesByID(toolbundle.DeriveBundles(s.registry.Definitions()))
		bundles = derived
	}
	var defs []llm.ToolDefinition
	for _, id := range bundleIDs {
		key := strings.ToLower(strings.TrimSpace(id))
		b := bundles[key]
		if b == nil && len(bundles) > 0 {
			// When workspace bundles exist but don't include the requested id,
			// fall back to derived bundles from tool registry.
			if derived == nil {
				derived = indexBundlesByID(toolbundle.DeriveBundles(s.registry.Definitions()))
			}
			b = derived[key]
		}
		if b == nil {
			appendWarning(ctx, fmt.Sprintf("unknown tool bundle: %s", id))
			continue
		}
		defs = append(defs, toolbundle.ResolveDefinitions(b, s.registry.MatchDefinition)...)
	}
	return dedupeDefinitions(defs), nil
}

func (s *Service) loadBundles(ctx context.Context) (map[string]*toolbundle.Bundle, error) {
	if s.toolBundles == nil {
		return nil, nil
	}
	list, err := s.toolBundles(ctx)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}
	return indexBundlesByID(list), nil
}

func indexBundlesByID(in []*toolbundle.Bundle) map[string]*toolbundle.Bundle {
	out := map[string]*toolbundle.Bundle{}
	for _, b := range in {
		if b == nil {
			continue
		}
		id := strings.TrimSpace(b.ID)
		if id == "" {
			continue
		}
		out[strings.ToLower(id)] = b
	}
	return out
}

func dedupeDefinitions(in []llm.ToolDefinition) []llm.ToolDefinition {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]llm.ToolDefinition, 0, len(in))
	for _, d := range in {
		key := strings.ToLower(mcpname.Canonical(strings.TrimSpace(d.Name)))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, d)
	}
	return out
}

func (s *Service) appendRegistryWarnings(ctx context.Context, tools []llm.Tool) []llm.Tool {
	if w, ok := s.registry.(interface {
		LastWarnings() []string
		ClearWarnings()
	}); ok {
		for _, msg := range w.LastWarnings() {
			appendWarning(ctx, msg)
		}
		w.ClearWarnings()
	}
	return tools
}
