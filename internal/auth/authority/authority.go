package authority

import (
    "net/url"
    "strings"
)

// AuthAuthority represents an authorization authority using issuer and/or origin.
// Either field may be empty if unknown. Normalize before comparing.
type AuthAuthority struct {
    Issuer string // e.g. https://idp.example.com/realms/acme
    Origin string // e.g. https://idp.example.com
}

// Normalize returns a copy of the authority with normalized Issuer and Origin.
// - Issuer: lowercased scheme/host, preserved path, no trailing slash
// - Origin: scheme://host[:port], lowercased scheme/host, default ports elided
func (a AuthAuthority) Normalize() AuthAuthority {
    n := a
    if n.Issuer != "" {
        n.Issuer = normalizeIssuer(n.Issuer)
    }
    if n.Origin != "" {
        n.Origin = normalizeOrigin(n.Origin)
    } else if n.Issuer != "" {
        if o := issuerOrigin(n.Issuer); o != "" {
            n.Origin = o
        }
    }
    return n
}

// SameAuthAuthority compares two authorities.
// Rules:
// - If both have Issuer, match normalized Issuer exactly (including path).
// - Else, match by Origin (scheme+host+port) if both present or derivable.
func SameAuthAuthority(a, b AuthAuthority) bool {
    na := a.Normalize()
    nb := b.Normalize()
    if na.Issuer != "" && nb.Issuer != "" {
        return na.Issuer == nb.Issuer
    }
    if na.Origin != "" && nb.Origin != "" {
        return na.Origin == nb.Origin
    }
    return false
}

// IsLocalAuth reports whether the target shares the same authority as the app.
func IsLocalAuth(app AuthAuthority, target AuthAuthority) bool {
    return SameAuthAuthority(app, target)
}

// AllowedAuthHeader reports whether an Authorization header can be sent to the target origin.
// The allowlist is compared after normalizing origins. Exact match only.
func AllowedAuthHeader(targetOrigin string, allowlist []string) bool {
    to := normalizeOrigin(targetOrigin)
    if to == "" {
        return false
    }
    for _, v := range allowlist {
        if to == normalizeOrigin(v) {
            return true
        }
    }
    return false
}

func normalizeIssuer(issuer string) string {
    u, err := url.Parse(issuer)
    if err != nil || u.Scheme == "" || u.Host == "" {
        return ""
    }
    u.Scheme = strings.ToLower(u.Scheme)
    u.Host = strings.ToLower(u.Host)
    // remove trailing slash on path
    if strings.HasSuffix(u.Path, "/") && len(u.Path) > 1 {
        u.Path = strings.TrimRight(u.Path, "/")
    }
    // keep path as-is (issuer paths matter, e.g., realms)
    // elide default ports from Host
    u.Host = elideDefaultPort(u)
    u.RawQuery = ""
    u.Fragment = ""
    return u.String()
}

func issuerOrigin(issuer string) string {
    u, err := url.Parse(issuer)
    if err != nil || u.Scheme == "" || u.Host == "" {
        return ""
    }
    return originFromURL(u)
}

func normalizeOrigin(origin string) string {
    u, err := url.Parse(origin)
    if err != nil || u.Scheme == "" || u.Host == "" {
        return ""
    }
    return originFromURL(u)
}

func originFromURL(u *url.URL) string {
    scheme := strings.ToLower(u.Scheme)
    host := strings.ToLower(u.Host)
    // construct origin as scheme://host[:port]
    // normalize/elide default ports
    host = elideDefaultPort(&url.URL{Scheme: scheme, Host: host})
    return scheme + "://" + host
}

func elideDefaultPort(u *url.URL) string {
    host := u.Host
    // If host already has no port, nothing to do
    if !strings.Contains(host, ":") {
        return host
    }
    // Split host:port
    h, p, ok := strings.Cut(host, ":")
    if !ok {
        return strings.ToLower(host)
    }
    // Default ports for http/https
    if (u.Scheme == "http" && p == "80") || (u.Scheme == "https" && p == "443") {
        return strings.ToLower(h)
    }
    return strings.ToLower(h + ":" + p)
}

