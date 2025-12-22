package expose

import (
	"context"
	"fmt"
	"net/http"

	"github.com/viant/agently/genai/executor"
	"github.com/viant/jsonrpc/transport"
	mcpclientproto "github.com/viant/mcp-protocol/client"
	mcplogger "github.com/viant/mcp-protocol/logger"
	mcpserverproto "github.com/viant/mcp-protocol/server"
	mcpserver "github.com/viant/mcp/server"
)

// NewHTTPServer constructs an MCP HTTP server exposing selected executor tools.
// It does not start listening; callers should run ListenAndServe and handle shutdown.
func NewHTTPServer(ctx context.Context, exec *executor.Service, cfg *executor.MCPServerConfig) (*http.Server, error) {
	if exec == nil {
		return nil, fmt.Errorf("mcp server: nil executor")
	}
	if cfg == nil {
		return nil, fmt.Errorf("mcp server: nil config")
	}
	if cfg.Port < 0 {
		return nil, fmt.Errorf("mcp server: invalid port %d", cfg.Port)
	}
	patterns := cfg.ToolPatterns()
	if cfg.Port != 0 && len(patterns) == 0 {
		return nil, fmt.Errorf("mcp server: tool.items patterns required when port is set")
	}

	h := NewToolHandler(exec, patterns)
	srv, err := mcpserver.New(
		mcpserver.WithRootRedirect(true),
		mcpserver.WithNewHandler(func(_ context.Context, _ transport.Notifier, _ mcplogger.Logger, _ mcpclientproto.Operations) (mcpserverproto.Handler, error) {
			return h, nil
		}),
	)
	if err != nil {
		return nil, err
	}
	srv.UseStreamableHTTP(true)

	addr := fmt.Sprintf("127.0.0.1:%d", cfg.Port)
	return srv.HTTP(ctx, addr), nil
}
