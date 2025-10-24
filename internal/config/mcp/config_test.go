package mcp

import (
    "context"
    "testing"
    "time"

    "github.com/viant/agently/internal/auth/shared"
)

func boolPtr(v bool) *bool { return &v }
func strPtr(s string) *string { return &s }

// No global defaults struct anymore; storage mapping defaults verified via ToTokenStoragePolicy

func TestResolveEffective_Precedence(t *testing.T) {
    ctx := context.Background()
    provider := ProviderAuth{}

    // built-in: no globals, no provider → reuse disabled, mode bearer_first
    reuse, mode := ResolveEffective(ctx, nil, nil, nil, nil, nil, nil, provider, nil, nil)
    if reuse || mode != shared.ModeBearerFirst {
        t.Fatalf("expected built-in false/bearer_first, got %v/%v", reuse, mode)
    }
    // provider overrides mode
    provider.ReuseAuthorizerMode = strPtr("cookie_first")
    _, mode = ResolveEffective(ctx, nil, nil, nil, nil, nil, nil, provider, nil, nil)
    if mode != shared.ModeCookieFirst {
        t.Fatalf("expected provider cookie_first, got %v", mode)
    }
    // client overrides both
    reuse, mode = ResolveEffective(ctx, boolPtr(false), strPtr("bearer_first"), nil, nil, nil, nil, provider, nil, nil)
    if reuse || mode != shared.ModeBearerFirst {
        t.Fatalf("expected client false/bearer_first, got %v/%v", reuse, mode)
    }
    // context overrides client
    ctx = shared.WithReuseAuthorizer(ctx, true)
    ctx = shared.WithReuseAuthorizerMode(ctx, shared.ModeCookieFirst)
    reuse, mode = ResolveEffective(ctx, boolPtr(false), strPtr("bearer_first"), nil, nil, nil, nil, provider, nil, nil)
    if !reuse || mode != shared.ModeCookieFirst {
        t.Fatalf("expected context true/cookie_first, got %v/%v", reuse, mode)
    }
    // env overrides provider
    reuse, mode = ResolveEffective(context.Background(), nil, nil, nil, nil, boolPtr(false), strPtr("bearer_first"), provider, nil, nil)
    if reuse || mode != shared.ModeBearerFirst {
        t.Fatalf("expected env false/bearer_first, got %v/%v", reuse, mode)
    }
    _ = time.Second
}
