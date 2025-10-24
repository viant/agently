package integrate

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/viant/agently/internal/auth/authority"
	"github.com/viant/agently/internal/auth/resolver"
	"github.com/viant/agently/internal/auth/tokens"
	"github.com/viant/jsonrpc/transport"
	mcpclient "github.com/viant/mcp/client"
)

// BootstrapConfig contains inputs to build an auth-enabled MCP client and SSE http client for a provider.
type BootstrapConfig struct {
	// Identity and transport
	Name      string
	Version   string
	Transport transport.Transport                                // required: underlying MCP transport already connected to provider
	Reconnect func(context.Context) (transport.Transport, error) // optional: to rebuild transport on reconnect

	// Authorities and allowlists
	AppAuthority      authority.AuthAuthority // app's OAuth/BFF authority
	MCPAuthority      authority.AuthAuthority // provider's MCP server authority
	OriginAllowlist   []string                // MCP server origins allowed to receive Authorization
	AudienceAllowlist []string                // allowed audiences for tokens
	AllowInsecure     bool                    // if true, allow auth over http (not recommended)
	// If true (default), fail bootstrap when AppAuthority and MCPAuthority do not match.
	// Set to false only if you intentionally allow non-reuse scenarios handled elsewhere.
	RequireSameAuthority bool

	// Token acquisition
	Resolver *resolver.Resolver // required: token resolver backed by BFF broker
	TokenKey tokens.Key         // required: identifies subject/audience under authority

	// HTTP/BFF infrastructure
	CookieJar        http.CookieJar    // shared jar for BFF authority (required for BFF exchange flows)
	BaseRoundTripper http.RoundTripper // optional: base HTTP transport
	RejectCacheTTL   time.Duration     // optional: suppress reuse of rejected ctx token
}

// ProviderClient bundles an MCP client with auth interceptor, an SSE http client with auth RoundTripper,
// and a token function for bearer-first injection.
type ProviderClient struct {
	Client    *mcpclient.Client
	SSEClient *http.Client
	TokenFn   func(context.Context) (string, time.Time, error)
	// Guards
	SameAuthority bool
}

// Bootstrap builds a ProviderClient with bearer-first auth behavior for both streamable and SSE transports.
func Bootstrap(cfg BootstrapConfig) (*ProviderClient, error) {
	if cfg.Transport == nil {
		return nil, errors.New("Bootstrap: Transport is required")
	}
	if cfg.Resolver == nil {
		return nil, errors.New("Bootstrap: Resolver is required")
	}
	if cfg.CookieJar == nil {
		return nil, errors.New("Bootstrap: CookieJar is required")
	}
	// Enforce authority match by default to avoid leaking Authorization to a different authority.
	same := authority.SameAuthAuthority(cfg.AppAuthority, cfg.MCPAuthority)
	if !cfg.AllowInsecure && same {
		// no-op: scheme check happens later per request; here we only compute same-ness
	}
	if cfg.RequireSameAuthority || (!cfg.RequireSameAuthority && cfg.RequireSameAuthority == false) {
		// default zero value is false; we want default true behavior, so flip when zero.
		if !cfg.RequireSameAuthority { // zero value → treat as true
			cfg.RequireSameAuthority = true
		}
	}
	if cfg.RequireSameAuthority && !same {
		return nil, errors.New("Bootstrap: authority mismatch; reuse disabled for this provider")
	}

	// Build auth RoundTripper for both HTTP metadata/exchange and SSE
	authRT, err := NewAuthRoundTripper(cfg.CookieJar, cfg.BaseRoundTripper, cfg.RejectCacheTTL)
	if err != nil {
		return nil, err
	}
	// Build MCP client with optional reconnect
	client := mcpclient.New(cfg.Name, cfg.Version, cfg.Transport)
	if cfg.Reconnect != nil {
		mcpclient.WithReconnect(func(ctx context.Context) (transport.Transport, error) {
			return cfg.Reconnect(ctx)
		})(client)
	}
	NewClientWithAuthInterceptor(client, authRT)
	// SSE http client shares the same auth RoundTripper
	sseHTTP := &http.Client{Transport: authRT}
	// Token function
	tokenFn := TokenFnFromResolver(cfg.Resolver, cfg.TokenKey)
	return &ProviderClient{
		Client:        client,
		SSEClient:     sseHTTP,
		TokenFn:       tokenFn,
		SameAuthority: same,
	}, nil
}
