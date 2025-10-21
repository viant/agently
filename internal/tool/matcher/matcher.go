package matcher

import "strings"

// Canon normalizes a tool name/pattern by trimming spaces and replacing
// both '/' and ':' with '_' for stable comparisons across different
// separators used in definitions and patterns.
func Canon(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, ":", "_")
	return s
}

// serverFromName returns the service/server portion from a canonical name like "svc_method" or "service:method".
func serverFromName(name string) string {
	name = strings.TrimSpace(name)
	if i := strings.IndexByte(name, ':'); i != -1 {
		return strings.TrimSpace(name[:i])
	}
	if i := strings.IndexByte(name, '_'); i != -1 {
		return strings.TrimSpace(name[:i])
	}
	return name
}

// Match returns true when pattern matches name using simple rules:
// - Exact match
// - '*' suffix means prefix match
// - Service-only pattern (no ':' and no '*') matches any method under that service
func Match(pattern, name string) bool {
	// Normalize both sides to a canonical form
	pcanon := Canon(pattern)
	ncanon := Canon(name)
	if pcanon == "*" {
		return true
	}
	if pcanon == ncanon {
		return true
	}
	if strings.Contains(pcanon, "*") {
		prefix := strings.TrimSuffix(pcanon, "*")
		return strings.HasPrefix(ncanon, prefix)
	}
	// Service-only pattern: when user provided just a service (no method)
	// Detect on the raw pattern to allow forms like "system/exec" or "system:exec".
	raw := strings.TrimSpace(pattern)
	if raw != "" && !strings.Contains(raw, ":") && !strings.Contains(raw, "*") {
		// Compare service portion only
		return strings.HasPrefix(ncanon, pcanon)
	}
	// Fallback: plain prefix match (legacy behavior)
	if strings.HasPrefix(ncanon, pcanon) {
		return true
	}
	return false
}
