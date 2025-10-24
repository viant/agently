package manager

import (
    "context"
    "net/http"
    "time"

    authctx "github.com/viant/agently/internal/auth"
    "github.com/viant/agently/internal/auth/authority"
    "github.com/viant/agently/internal/auth/mcp/integrate"
    "github.com/viant/agently/internal/auth/runtime"
    "github.com/viant/agently/internal/auth/tokens"
    mcpclient "github.com/viant/mcp/client"
)

// AuthIntegration holds dependencies and policy for bearer-first reuse on MCP clients.
type AuthIntegration struct {
    Runtime              *runtime.Runtime
    AppAuthority         authority.AuthAuthority
    Audience             string
    AllowInsecure        bool
    RequireSameAuthority bool
    BaseTransport        http.RoundTripper
    RejectCacheTTL       time.Duration
}

// attachAuthIfPossible attaches an auth Authorizer + RoundTripper to a concrete *mcpclient.Client.
// It is safe to call on any implementation of mcpclient.Interface; non-concrete types are ignored.
func attachAuthIfPossible(ctx context.Context, cli mcpclient.Interface, ai *AuthIntegration) error {
    if cli == nil || ai == nil || ai.Runtime == nil {
        return nil
    }
    // Resolve subject from context; without a user identity we skip attaching auth.
    subject := authctx.EffectiveUserID(ctx)
    if subject == "" {
        return nil
    }
    _ = tokens.Key{Authority: ai.AppAuthority, Subject: subject, Audience: ai.Audience}

    // Prepare resolver and shared cookie jar for the app authority
    if _, err := ai.Runtime.NewResolver(); err != nil {
        return err
    }
    jar, _, err := ai.Runtime.CookieJarForAuthority(ai.AppAuthority)
    if err != nil {
        return err
    }
    rt, err := integrate.NewAuthRoundTripper(jar, ai.BaseTransport, ai.RejectCacheTTL)
    if err != nil {
        return err
    }
    c, ok := cli.(*mcpclient.Client)
    if !ok {
        return nil
    }
    integrate.NewClientWithAuthInterceptor(c, rt)
    return nil
}

