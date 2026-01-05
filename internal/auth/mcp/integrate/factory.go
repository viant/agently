package integrate

import (
	"context"
	"net/http"
	"time"

	"github.com/viant/agently/internal/auth/resolver"
	"github.com/viant/agently/internal/auth/tokens"
	mcpclient "github.com/viant/mcp/client"
	"github.com/viant/mcp/client/auth"
	authtransport "github.com/viant/mcp/client/auth/transport"
)

// NewAuthRoundTripper builds an auth RoundTripper configured for BFF exchange and cookie reuse.
// Pass a shared cookie jar tied to the app's BFF authority. Optionally override base transport.
func NewAuthRoundTripper(jar http.CookieJar, base http.RoundTripper, rejectTTL time.Duration) (*authtransport.RoundTripper, error) {
	opts := []authtransport.Option{
		authtransport.WithBackendForFrontendAuth(),
		authtransport.WithCookieJar(jar),
	}
	// Note: cookie persistence is handled by the http.Client that uses this
	// RoundTripper. Ensure the client is constructed with Jar set to `jar`.
	if base == nil {
		base = http.DefaultTransport
	}
	if base != nil {
		opts = append(opts, authtransport.WithTransport(base))
	}
	return authtransport.New(opts...)
}

// NewAuthRoundTripperWithPrompt wraps the provided base transport with an OOB
// prompt trigger and builds the auth RoundTripper on top. When the resource
// responds 401 with an authorization_uri, the prompt is invoked (non-blocking),
// and the original response is returned to the caller.
func NewAuthRoundTripperWithPrompt(jar http.CookieJar, base http.RoundTripper, rejectTTL time.Duration, prompt OAuthPrompt) (*authtransport.RoundTripper, error) {
	if prompt != nil {
		base = NewOOBRoundTripper(base, prompt)
	}
	return NewAuthRoundTripper(jar, base, rejectTTL)
}

// NewClientWithAuthInterceptor attaches an Authorizer that auto-retries once on 401.
// Provide a pre-built transport.Transport when constructing the MCP client elsewhere; this helper
// only binds the interceptor to the MCP client.
func NewClientWithAuthInterceptor(client *mcpclient.Client, rt *authtransport.RoundTripper) *mcpclient.Client {
	if client == nil || rt == nil {
		return client
	}
	authorizer := auth.NewAuthorizer(rt)
	// mimic option application
	mcpclient.WithAuthInterceptor(authorizer)(client)
	return client
}

// ContextWithAuthToken returns a context that carries a bearer token for the auth RoundTripper.
func ContextWithAuthToken(ctx context.Context, token string) context.Context {
	if token == "" {
		return ctx
	}
	return context.WithValue(ctx, authtransport.ContextAuthTokenKey, token)
}

// WithTokenOption returns a request option that injects the bearer token for the MCP request.
func WithTokenOption(token string) mcpclient.RequestOption {
	return mcpclient.WithAuthToken(token)
}

// TokenFnFromResolver adapts a Resolver + Key into a token function usable by
// streaming helpers. It returns the access token and its expiry.
func TokenFnFromResolver(r *resolver.Resolver, key tokens.Key) func(context.Context) (string, time.Time, error) {
	return func(ctx context.Context) (string, time.Time, error) {
		tok, err := r.EnsureAccess(ctx, key)
		if err != nil {
			return "", time.Time{}, err
		}
		return tok.AccessToken, tok.Expiry, nil
	}
}
