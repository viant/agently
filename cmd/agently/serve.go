package agently

import (
	root "github.com/viant/agently"
)

// ServeCmd starts the HTTP server.
type ServeCmd struct {
	Addr      string `short:"a" long:"addr" description:"listen address" default:":8080"`
	Policy    string `short:"p" long:"policy" description:"tool policy: auto|ask|deny" default:"auto"`
	Workspace string `short:"w" long:"workspace" description:"workspace root path (overrides AGENTLY_WORKSPACE when set)"`
	ExposeMCP bool   `long:"expose-mcp" description:"Expose Agently tools over an MCP HTTP server (requires mcpServer.port and tool patterns in config)"`
	UIDist    string `long:"ui-dist" description:"Optional local UI dist directory override"`
	Debug     bool   `short:"d" long:"debug" description:"Enable debug mode"`
}

func (c *ServeCmd) Execute(_ []string) error {
	return root.Serve(root.ServeOptions{
		Addr:          c.Addr,
		WorkspacePath: c.Workspace,
		UIDist:        c.UIDist,
		Debug:         c.Debug,
		Policy:        c.Policy,
		ExposeMCP:     c.ExposeMCP,
	})
}
