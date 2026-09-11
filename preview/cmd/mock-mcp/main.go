// mock-mcp serves JSON fixtures for any MCP-backed Forge datasource.
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/viant/agently/preview/mcp/mock"
	"github.com/viant/agently/preview/report"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	root := flag.String("root", "", "fixture folder containing ds/*.json")
	addr := flag.String("addr", "127.0.0.1:8097", "loopback listen address")
	variant := flag.String("variant", "default", "default fixture variant")
	dataRoot := flag.String("data-root", "ds", "relative base fixture directory")
	report := flag.Bool("report", false, "use report.yaml schemas and report query capabilities")
	flag.Parse()
	if *root == "" {
		return fmt.Errorf("--root is required")
	}
	host, _, err := net.SplitHostPort(*addr)
	if err != nil {
		return err
	}
	if host != "localhost" && (net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback()) {
		return fmt.Errorf("mock server must bind to loopback")
	}
	var server *mock.Server
	if *report {
		server, err = reportpreview.MockServer(reportpreview.Config{Folder: *root, Variant: *variant})
	} else {
		server, err = mock.New(mock.Config{Root: *root, DataRoot: *dataRoot, Variant: *variant})
	}
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: mock.LocalOnly(server.HTTPHandler()), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 128 << 10}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	go func() {
		<-ctx.Done()
		shutdown, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_ = srv.Shutdown(shutdown)
	}()
	fmt.Fprintf(os.Stdout, "Synthetic fixture MCP server: http://%s/mcp\n", listener.Addr())
	err = srv.Serve(listener)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}
