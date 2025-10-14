package agent

import (
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
		out = append(out, pattern)
	}
	return out
}

// resolveTools resolves agent tool patterns to concrete tools and optionally
// applies an allow-list filter based on input.ToolsAllowed.
func (s *Service) resolveTools(qi *QueryInput, applyAllow bool) ([]llm.Tool, error) {
	if len(qi.ToolsAllowed) == 0 && qi.ToolsAllowed != nil {
		return []llm.Tool{}, nil
	}
	patterns := toolPatterns(qi)
	if len(patterns) == 0 {
		return nil, nil
	}
	tools, err := s.registry.MustHaveTools(patterns)
	if err != nil {
		return nil, err
	}
	if !applyAllow || qi.ToolsAllowed == nil {
		return tools, nil
	}

	allowed := map[string]bool{}
	for _, n := range qi.ToolsAllowed {
		if n = strings.TrimSpace(n); n != "" {
			allowed[n] = true
		}
	}
	var filtered []llm.Tool
	for _, t := range tools {
		name := strings.TrimSpace(t.Definition.Name)
		if name == "" {
			continue
		}
		if allowed[name] {
			filtered = append(filtered, t)
		}
	}
	return filtered, nil
}
