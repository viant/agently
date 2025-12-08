package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/viant/agently/genai/llm"
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
//   - If input.ToolsAllowed is provided (non-nil), resolve exactly those tools by name
//     and do not gate by agent patterns.
//   - Otherwise, resolve tools from agent patterns.
func (s *Service) resolveTools(ctx context.Context, qi *QueryInput) ([]llm.Tool, error) {
	// Clear any previous registry warnings before this resolution cycle.
	if w, ok := s.registry.(interface{ ClearWarnings() }); ok {
		w.ClearWarnings()
	}
	// Prefer explicit allow-list when provided (even if empty).
	if qi.ToolsAllowed != nil {
		if len(qi.ToolsAllowed) == 0 {
			return []llm.Tool{}, nil
		}
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
