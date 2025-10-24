package shared

import (
    "context"
    "strings"
)

// Built-in defaults when nothing is configured.
const (
    // BuiltInDefaultReuseAuthorizer controls behavior when no source (context/client/CLI/env/provider/global) sets reuse.
    // Set to false so reuse is disabled unless explicitly enabled.
    BuiltInDefaultReuseAuthorizer = false
)

// Mode represents how to apply reused auth to MCP requests.
type Mode string

const (
    ModeBearerFirst Mode = "bearer_first"
    ModeCookieFirst Mode = "cookie_first"
)

// Built-in default mode.
const BuiltInDefaultMode = ModeBearerFirst

// normalizeMode parses various user inputs to a Mode.
// Accepts case-insensitive values and hyphen/underscore variants.
func normalizeMode(v string) (Mode, bool) {
    if v == "" {
        return "", false
    }
    s := strings.ToLower(strings.TrimSpace(v))
    s = strings.ReplaceAll(s, "-", "_")
    switch s {
    case string(ModeBearerFirst):
        return ModeBearerFirst, true
    case string(ModeCookieFirst):
        return ModeCookieFirst, true
    default:
        return "", false
    }
}

// context keys
type reuseAuthKey struct{}
type reuseModeKey struct{}

// WithReuseAuthorizer sets a per-request override.
func WithReuseAuthorizer(ctx context.Context, v bool) context.Context {
    return context.WithValue(ctx, reuseAuthKey{}, &v)
}

// ReuseAuthorizerFromContext returns the override if present.
func ReuseAuthorizerFromContext(ctx context.Context) (*bool, bool) {
    v, ok := ctx.Value(reuseAuthKey{}).(*bool)
    return v, ok
}

// WithReuseAuthorizerMode sets a per-request mode override.
func WithReuseAuthorizerMode(ctx context.Context, m Mode) context.Context {
    v := m
    return context.WithValue(ctx, reuseModeKey{}, &v)
}

// ReuseAuthorizerModeFromContext returns the mode override if present.
func ReuseAuthorizerModeFromContext(ctx context.Context) (*Mode, bool) {
    v, ok := ctx.Value(reuseModeKey{}).(*Mode)
    return v, ok
}

// ReuseAuthorizerResolutionInput aggregates potential sources for the setting.
type ReuseAuthorizerResolutionInput struct {
    Ctx          context.Context
    ClientOpt    *bool  // MCPReuseAuthorizer pointer; nil if unspecified
    CLIOpt       *bool  // CLI flag override; nil if unspecified
    EnvOpt       *bool  // Env var override; nil if unspecified
    ProviderOpt  *bool  // Provider config; nil if unspecified
    GlobalOpt    *bool  // Global config default; nil if unspecified
}

// ResolveReuseAuthorizer applies precedence to determine the effective setting.
// Precedence: context > client > CLI > env > provider > global > built-in.
func ResolveReuseAuthorizer(in ReuseAuthorizerResolutionInput) bool {
    if v, ok := ReuseAuthorizerFromContext(in.Ctx); ok && v != nil {
        return *v
    }
    if in.ClientOpt != nil {
        return *in.ClientOpt
    }
    if in.CLIOpt != nil {
        return *in.CLIOpt
    }
    if in.EnvOpt != nil {
        return *in.EnvOpt
    }
    if in.ProviderOpt != nil {
        return *in.ProviderOpt
    }
    if in.GlobalOpt != nil {
        return *in.GlobalOpt
    }
    return BuiltInDefaultReuseAuthorizer
}

// ReuseModeResolutionInput aggregates potential sources for the mode.
type ReuseModeResolutionInput struct {
    Ctx          context.Context
    ClientOpt    *string // MCPReuseAuthorizerMode pointer; nil if unspecified
    CLIOpt       *string // CLI flag override; nil if unspecified
    EnvOpt       *string // Env var override; nil if unspecified
    ProviderOpt  *string // Provider config; nil if unspecified
    GlobalOpt    *string // Global config default; nil if unspecified
}

// ResolveReuseAuthorizerMode resolves the mode with precedence and normalization.
// Precedence: context > client > CLI > env > provider > global > built-in.
func ResolveReuseAuthorizerMode(in ReuseModeResolutionInput) Mode {
    if mv, ok := ReuseAuthorizerModeFromContext(in.Ctx); ok && mv != nil {
        return *mv
    }
    if in.ClientOpt != nil {
        if m, ok := normalizeMode(*in.ClientOpt); ok {
            return m
        }
    }
    if in.CLIOpt != nil {
        if m, ok := normalizeMode(*in.CLIOpt); ok {
            return m
        }
    }
    if in.EnvOpt != nil {
        if m, ok := normalizeMode(*in.EnvOpt); ok {
            return m
        }
    }
    if in.ProviderOpt != nil {
        if m, ok := normalizeMode(*in.ProviderOpt); ok {
            return m
        }
    }
    if in.GlobalOpt != nil {
        if m, ok := normalizeMode(*in.GlobalOpt); ok {
            return m
        }
    }
    return BuiltInDefaultMode
}
