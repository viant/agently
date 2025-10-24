package http

import (
    "context"
    "fmt"
    "time"

    "github.com/viant/agently/internal/auth/authority"
    "github.com/viant/agently/internal/auth/resolver"
    "github.com/viant/agently/internal/auth/shared"
    "github.com/viant/agently/internal/auth/tokens"
    "github.com/viant/agently/internal/obs"
    "net/url"
)

// AuthParams encapsulates inputs required to build an auth header for MCP requests.
type AuthParams struct {
    // Resolution inputs
    Ctx            context.Context
    ReuseInput     shared.ReuseAuthorizerResolutionInput
    ModeInput      shared.ReuseModeResolutionInput

    // Target and policy
    TargetOrigin   string
    Allowlist      []string
    AppAuthority   authority.AuthAuthority
    MCPAuthority   authority.AuthAuthority

    // Token resolution
    TokenKey       tokens.Key
    Resolver       *resolver.Resolver

    // Observability
    Tracer         obs.Tracer
    Metrics        obs.Metrics

    // Security hardening
    AudienceAllowlist []string // if set, TokenKey.Audience must be in this list
    AllowInsecure     bool     // default false; if false, do not send Authorization to non-HTTPS origins
}

// BuildAuthHeader bearer-first (by mode) when reuse is enabled and target origin is allowed.
// Returns header key and value when applied, otherwise ok=false.
func BuildAuthHeader(p AuthParams) (key string, value string, ok bool, err error) {
    tracer := p.Tracer
    metrics := p.Metrics
    if tracer == nil { tracer = obs.NoopTracer{} }
    if metrics == nil { metrics = obs.NoopMetrics{} }
    reuse := shared.ResolveReuseAuthorizer(p.ReuseInput)
    if !reuse {
        tracer.Debug("auth_reuse_disabled", map[string]any{"origin": p.TargetOrigin})
        return "", "", false, nil
    }
    mode := shared.ResolveReuseAuthorizerMode(p.ModeInput)
    if mode != shared.ModeBearerFirst {
        tracer.Debug("auth_mode_not_bearer_first", map[string]any{"mode": string(mode)})
        return "", "", false, nil
    }
    if !authority.AllowedAuthHeader(p.TargetOrigin, p.Allowlist) {
        tracer.Debug("auth_origin_not_allowed", map[string]any{"origin": p.TargetOrigin})
        return "", "", false, nil
    }
    // Enforce HTTPS unless explicitly allowed
    if !p.AllowInsecure {
        if u, perr := urlParse(p.TargetOrigin); perr != nil || u.Scheme != "https" {
            tracer.Debug("auth_scheme_blocked", map[string]any{"origin": p.TargetOrigin})
            return "", "", false, nil
        }
    }
    if !authority.SameAuthAuthority(p.AppAuthority, p.MCPAuthority) {
        // For safety, require authority mapping to match for reuse
        tracer.Debug("auth_authority_mismatch", map[string]any{"app": p.AppAuthority.Normalize().Origin, "mcp": p.MCPAuthority.Normalize().Origin})
        return "", "", false, nil
    }
    // Audience allowlist if provided
    if len(p.AudienceAllowlist) > 0 {
        allowed := false
        for _, a := range p.AudienceAllowlist {
            if a == p.TokenKey.Audience { allowed = true; break }
        }
        if !allowed {
            tracer.Debug("auth_audience_not_allowed", map[string]any{"audience": p.TokenKey.Audience})
            return "", "", false, nil
        }
    }
    if p.Resolver == nil {
        return "", "", false, fmt.Errorf("resolver is required in bearer_first mode")
    }
    // Ensure we have a valid access token before the first attempt
    tok, err := p.Resolver.EnsureAccess(p.Ctx, p.TokenKey)
    if err != nil {
        return "", "", false, err
    }
    acc := tok.AccessToken
    tracer.Debug("auth_header_bearer_first", map[string]any{"origin": p.TargetOrigin})
    metrics.Inc("auth_header_bearer", map[string]string{"origin": p.TargetOrigin}, 1)
    return "Authorization", "Bearer " + acc, true, nil
}

// urlParse is a wrapper for testing; defined to avoid importing net/url in callers unexpectedly.
var urlParse = func(raw string) (*url.URL, error) { return url.Parse(raw) }

// MinTokenTTL returns a conservative minTTL for early refresh.
func MinTokenTTL(ttl time.Duration, def time.Duration) time.Duration {
    if ttl <= 0 {
        return def
    }
    return ttl
}
