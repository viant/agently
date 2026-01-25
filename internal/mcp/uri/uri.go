package uri

import (
	neturl "net/url"
	"path"
	"strings"
)

// Is reports whether the URI uses the MCP scheme (mcp: or mcp://).
func Is(uri string) bool {
	return strings.HasPrefix(uri, "mcp:") || strings.HasPrefix(uri, "mcp://")
}

// Parse splits an MCP URI into (server, resourceURI). Supports:
// - mcp://server/path
// - mcp:server:/path
// - mcp:server://<resource>
// When parsing fails, empty strings are returned.
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
	if i := strings.Index(raw, "://"); i > 0 && !strings.Contains(raw[:i], ":") {
		server = raw[:i]
		resource := strings.TrimSpace(raw[i+3:])
		if resource == "" {
			return server, ""
		}
		if strings.Contains(resource, "://") {
			return server, resource
		}
		return server, server + "://" + resource
	}
	// Support legacy "mcp:<server>:<resource-uri>" form by passing through the
	// full resource URI.
	if strings.Contains(raw, "://") {
		if i := strings.IndexByte(raw, ':'); i > 0 {
			return raw[:i], raw[i+1:]
		}
	}
	if i := strings.IndexByte(raw, ':'); i != -1 {
		return raw[:i], raw[i+1:]
	}
	if j := strings.IndexByte(raw, '/'); j != -1 {
		return raw[:j], raw[j:]
	}
	return "", ""
}

// Canonical builds a normalized MCP URI in the form:
// mcp:<server>://<resource>
// If resourceURI already carries the same scheme as the server, the scheme
// is stripped from the canonical form (e.g. github://host/path -> host/path).
func Canonical(server, resourceURI string) string {
	srv := strings.TrimSpace(server)
	if srv == "" {
		return ""
	}
	res := strings.TrimSpace(resourceURI)
	if res == "" {
		return "mcp:" + srv + "://"
	}
	if strings.Contains(res, "://") {
		if u, err := neturl.Parse(res); err == nil && strings.EqualFold(u.Scheme, srv) {
			host := strings.TrimSpace(u.Host)
			path := strings.TrimPrefix(u.EscapedPath(), "/")
			res = strings.TrimPrefix(strings.TrimPrefix(host+"/"+path, "/"), "/")
			if u.RawQuery != "" {
				res += "?" + u.RawQuery
			}
		}
	}
	if strings.HasPrefix(res, "/") {
		res = strings.TrimPrefix(res, "/")
	}
	return "mcp:" + srv + "://" + res
}

// JoinResourcePath appends a relative path to a resource URI.
// It preserves the scheme/host when the base URI is fully qualified.
func JoinResourcePath(baseURI, rel string) string {
	base := strings.TrimSpace(baseURI)
	if base == "" {
		return strings.TrimPrefix(rel, "/")
	}
	rel = strings.TrimPrefix(rel, "/")
	if strings.Contains(base, "://") {
		if u, err := neturl.Parse(base); err == nil {
			u.Path = path.Join(u.Path, rel)
			return u.String()
		}
	}
	return path.Join(base, rel)
}

// NormalizeForCompare returns a canonical MCP URI string for comparisons.
// It collapses legacy forms into mcp:<server>://<resource> and trims trailing slashes.
func NormalizeForCompare(value string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return ""
	}
	if !Is(v) {
		return strings.TrimRight(v, "/")
	}
	server, uri := Parse(v)
	if strings.TrimSpace(server) == "" {
		return strings.TrimRight(v, "/")
	}
	normalized := Canonical(server, uri)
	return strings.TrimRight(normalized, "/")
}

// CompareParts returns the MCP server and a set of normalized resource paths
// suitable for matching roots across shorthand and fully-qualified forms.
// For example:
// - mcp://github/owner/repo -> server=github, paths=["/owner/repo"]
// - mcp:github:github://github.vianttech.com/owner/repo -> server=github, paths=["/owner/repo"]
// - mcp:github:github://owner/repo -> server=github, paths=["/owner/repo"]
func CompareParts(value string) (server string, paths []string) {
	raw := strings.TrimSpace(value)
	if raw == "" || !Is(raw) {
		return "", nil
	}
	server, uri := Parse(raw)
	server = strings.TrimSpace(server)
	uri = strings.TrimSpace(uri)
	if server == "" || uri == "" {
		return server, nil
	}
	ensureSlash := func(p string) string {
		if p == "" {
			return "/"
		}
		if !strings.HasPrefix(p, "/") {
			return "/" + p
		}
		return p
	}
	if strings.Contains(uri, "://") {
		if u, err := neturl.Parse(uri); err == nil {
			host := strings.TrimSpace(u.Host)
			path := ensureSlash(strings.TrimSpace(u.Path))
			if host != "" && !strings.Contains(host, ".") {
				paths = append(paths, ensureSlash(host+path))
			} else {
				paths = append(paths, path)
			}
			return server, paths
		}
	}
	trimmed := strings.TrimPrefix(uri, "/")
	if trimmed != "" {
		if i := strings.IndexByte(trimmed, '/'); i != -1 {
			host := trimmed[:i]
			rest := trimmed[i+1:]
			if strings.Contains(host, ".") {
				paths = append(paths, ensureSlash(rest))
				return server, paths
			}
		} else if strings.Contains(trimmed, ".") {
			paths = append(paths, "/")
			return server, paths
		}
	}
	paths = append(paths, ensureSlash(uri))
	return server, paths
}
