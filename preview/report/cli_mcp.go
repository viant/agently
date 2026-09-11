package reportpreview

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/viant/agently/preview/mcp/mock"
	preview "github.com/viant/agently/preview/report/provider"
)

// connectCLI gives offline CLI invocations the same real MCP transport as the
// browser host, using an ephemeral loopback port and closing it after the command.
func connectCLI(ctx context.Context, p *preview.Package, folder, variant string) (func(), error) {
	server, err := MockServer(Config{Folder: folder, Variant: variant})
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	httpServer := &http.Server{Handler: mock.LocalOnly(server.HTTPHandler()), ReadHeaderTimeout: 5 * time.Second}
	go httpServer.Serve(listener)
	close := func() { _ = httpServer.Close() }
	if err = p.UseMCP(ctx, "http://"+listener.Addr().String()); err != nil {
		close()
		return nil, err
	}
	return close, nil
}
