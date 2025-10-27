package proxy

import (
	"context"
	"strings"

	authctx "github.com/viant/agently/internal/auth"
	mcpschema "github.com/viant/mcp-protocol/schema"
	mcpclient "github.com/viant/mcp/client"
)

// Proxy wraps an MCP client to normalize tool names and provide simple helpers.
type Proxy struct {
	cli    mcpclient.Interface
	server string
}

// NewProxy constructs a proxy bound to a specific server name.
func NewProxy(_ context.Context, server string, cli mcpclient.Interface) (*Proxy, error) {
	if cli == nil {
		return nil, nil
	}
	return &Proxy{cli: cli, server: strings.TrimSpace(server)}, nil
}

// CallTool normalizes name and dispatches to the underlying client.
func (p *Proxy) CallTool(ctx context.Context, name string, args map[string]interface{}, opts ...mcpclient.RequestOption) (*mcpschema.CallToolResult, error) {
	call := normalizeToolName(p.server, strings.TrimSpace(name))
	// Attach bearer when available (cookies are handled by the client's transport cookie jar)
	if tok := strings.TrimSpace(authctx.Bearer(ctx)); tok != "" {
		opts = append(opts, mcpclient.WithAuthToken(tok))
	}
	res, err := p.cli.CallTool(ctx, &mcpschema.CallToolRequestParams{Name: call, Arguments: args}, opts...)
	return res, err
}

// ListAllTools returns all tools for the server by paging through cursors.
func (p *Proxy) ListAllTools(ctx context.Context) ([]mcpschema.Tool, error) {
	var (
		tools  []mcpschema.Tool
		cursor *string
	)
	for {
		// Do not inject bearer here; rely on MCP client's auth transport
		// (cookie jar or interactive flow) to authenticate discovery.
		res, err := p.cli.ListTools(ctx, cursor, authOption(ctx)...)
		if err != nil {
			return nil, err
		}
		tools = append(tools, res.Tools...)
		if res.NextCursor == nil || *res.NextCursor == "" {
			break
		}
		cursor = res.NextCursor
	}
	return tools, nil
}

// authOption returns a RequestOption carrying Authorization Bearer when present in ctx.
func authOption(ctx context.Context) []mcpclient.RequestOption {
	tok := strings.TrimSpace(authctx.Bearer(ctx))
	if tok == "" {
		return nil
	}
	return []mcpclient.RequestOption{mcpclient.WithAuthToken(tok)}
}

func normalizeToolName(server, name string) string {
	if name == "" {
		return name
	}
	// server/method → method (MCP expects method scoped to this server)
	if i := strings.IndexByte(name, '/'); i != -1 {
		// Only strip when the prefix matches our server; otherwise leave as-is
		if strings.TrimSpace(server) == strings.TrimSpace(name[:i]) {
			return name[i+1:]
		}
		return name[i+1:]
	}
	// service-method canonical → method when prefix matches
	if i := strings.LastIndexByte(name, '-'); i != -1 {
		return name[i+1:]
	}
	return name
}
