package mcpuri

import (
	neturl "net/url"
	"strings"
)

// Is reports whether the URI uses the MCP scheme (mcp: or mcp://).
func Is(uri string) bool {
	return strings.HasPrefix(uri, "mcp:") || strings.HasPrefix(uri, "mcp://")
}

// Parse splits an MCP URI into (server, resourceURI). Supports both
// mcp://server/path and mcp:server:/path formats. When parsing fails,
// empty strings are returned.
func Parse(src string) (server, uri string) {
	if strings.HasPrefix(src, "mcp://") {
		if u, err := neturl.Parse(src); err == nil {
			server = u.Host
			uri = u.EscapedPath()
			if u.RawQuery != "" {
				uri += "?" + u.RawQuery
			}
			return
		}
	}
	raw := strings.TrimPrefix(src, "mcp:")
	if i := strings.IndexByte(raw, ':'); i != -1 {
		return raw[:i], raw[i+1:]
	}
	if j := strings.IndexByte(raw, '/'); j != -1 {
		return raw[:j], raw[j:]
	}
	return "", ""
}
