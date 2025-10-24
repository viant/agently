package integrate

import (
    "context"
    "time"

    mcpclient "github.com/viant/mcp/client"
    "github.com/viant/mcp-protocol/schema"
)

// SubscribeWithAuth performs a streamable subscription with bearer-first auth.
// It acquires a token via tokenFn, attaches it using WithAuthToken, and calls the client's Subscribe.
// The caller should configure the MCP client with an auth Authorizer + RoundTripper so a 401 at
// handshake triggers a single automatic retry.
func SubscribeWithAuth(ctx context.Context, client *mcpclient.Client, params *schema.SubscribeRequestParams, tokenFn func(context.Context) (string, time.Time, error)) (*schema.SubscribeResult, error) {
    token, _, err := tokenFn(ctx)
    if err != nil {
        return nil, err
    }
    // Optionally place token into context for transports that use it
    ctx = ContextWithAuthToken(ctx, token)
    return client.Subscribe(ctx, params, mcpclient.WithAuthToken(token))
}
