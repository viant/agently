package manager

import (
	"context"
	"strings"

	authctx "github.com/viant/agently/internal/auth"
	authtransport "github.com/viant/mcp/client/auth/transport"
)

// UseIDToken reports whether the MCP server config prefers using an ID token
// when authenticating outbound calls to this server.
func (m *Manager) UseIDToken(ctx context.Context, serverName string) bool {
	if m == nil {
		return false
	}
	name := strings.TrimSpace(serverName)
	if name == "" {
		return false
	}
	cfg, err := m.Options(ctx, name)
	if err != nil || cfg == nil || cfg.ClientOptions == nil || cfg.ClientOptions.Auth == nil {
		return false
	}
	return cfg.ClientOptions.Auth.UseIdToken
}

// WithAuthTokenContext injects the selected auth token into context under the
// MCP auth transport key so HTTP transports can emit the appropriate Bearer header.
// Respects the per-server PassUserToken config: when explicitly set to false,
// the user's token is NOT forwarded. When nil (default), the token IS forwarded.
func (m *Manager) WithAuthTokenContext(ctx context.Context, serverName string) context.Context {
	if ctx == nil || m == nil {
		return ctx
	}
	// Check PassUserToken config — skip when explicitly disabled.
	name := strings.TrimSpace(serverName)
	if name != "" {
		if cfg, err := m.Options(ctx, name); err == nil && cfg != nil && cfg.ClientOptions != nil {
			if cfg.ClientOptions.Auth != nil && !cfg.ClientOptions.Auth.ShouldPassUserToken() {
				return ctx
			}
		}
	}
	useID := m.UseIDToken(ctx, serverName)
	tok := authctx.MCPAuthToken(ctx, useID)
	if strings.TrimSpace(tok) == "" {
		return ctx
	}
	return context.WithValue(ctx, authtransport.ContextAuthTokenKey, tok)
}
