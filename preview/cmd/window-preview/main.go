package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/viant/agently/preview/window"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	root := flag.String("root", "preview/window/examples/projects", "preview workspace")
	metadataRoot := flag.String("metadata-root", "", "optional Forge metadata root; defaults to --root")
	mcpURL := flag.String("mcp-url", "", "optional stable loopback MCP URL override")
	mcpBaseURL := flag.String("mcp-base-url", "", "deprecated alias for --mcp-url")
	platformMCPURL := flag.String("platform-mcp-url", "", "stable loopback Platform MCP URL override")
	stewardMCPURL := flag.String("steward-mcp-url", "", "stable loopback Steward MCP URL override")
	disableAuthorization := flag.Bool("disable-authorization", false, "omit window authorization preflight for standalone preview")
	readOnly := flag.Bool("read-only", false, "disable workspace actions and mutations for standalone preview")
	addr := flag.String("addr", "127.0.0.1:8098", "loopback address")
	assets := flag.String("assets", "preview/ui/dist", "built frontend directory")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if *mcpURL != "" && *mcpBaseURL != "" && *mcpURL != *mcpBaseURL {
		fmt.Fprintln(os.Stderr, "--mcp-url and --mcp-base-url must match when both are supplied")
		os.Exit(2)
	}
	if *mcpURL == "" {
		*mcpURL = *mcpBaseURL
	}
	if e := windowpreview.Serve(ctx, windowpreview.Config{Root: *root, MetadataRoot: *metadataRoot, MCPURL: *mcpURL, PlatformMCPURL: *platformMCPURL, StewardMCPURL: *stewardMCPURL, DisableAuthorization: *disableAuthorization, ReadOnly: *readOnly, Addr: *addr, Assets: *assets}, os.Stdout); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
