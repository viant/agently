package agently

import (
	"fmt"
	root "github.com/viant/agently"
)

// ServeCmd starts the HTTP server.
type ServeCmd struct {
	DisableBackgroundWorkers bool   `long:"disable-background-workers" description:"Disable autonomous/scheduled work on an isolated interactive validation host"`
	ValidateAuthz            bool   `long:"validate-authz" description:"Validate explicit workspace authorization configuration without starting a server"`
	AuthzConfig              string `long:"authz-config" description:"Trusted unified authorization JSON configuration (or AGENTLY_AUTHZ_CONFIG)"`
	Addr                     string `short:"a" long:"addr" description:"listen address" default:":8080"`
	Policy                   string `short:"p" long:"policy" description:"tool policy: auto|ask|deny" default:"auto"`
	Workspace                string `short:"w" long:"workspace" description:"workspace root path (overrides AGENTLY_WORKSPACE when set)"`
	ScratchpadRootURI        string `short:"s" long:"scratchpad-root-uri" description:"User-scoped scratchpad URI template (overrides AGENTLY_SCRATCHPAD_URI when set)"`
	ExposeMCP                bool   `long:"expose-mcp" description:"Expose Agently tools over an MCP HTTP server (requires mcpServer.port and tool patterns in config)"`
	UIDist                   string `long:"ui-dist" description:"Optional local UI dist directory override"`
	Debug                    bool   `short:"d" long:"debug" description:"Enable debug mode"`
}

func (c *ServeCmd) Execute(_ []string) error {
	if c.ValidateAuthz {
		if err := root.ValidateHostAuthorizationConfiguration(c.serveOptions()); err != nil {
			return err
		}
		fmt.Println("Authorization configuration validated; no server started")
		return nil
	}
	return root.Serve(c.serveOptions())
}

func (c *ServeCmd) serveOptions() root.ServeOptions {
	return root.ServeOptions{
		DisableBackgroundWorkers: c.DisableBackgroundWorkers,
		AuthzConfigPath:          c.AuthzConfig,
		Addr:                     c.Addr,
		WorkspacePath:            c.Workspace,
		ScratchpadRootURI:        c.ScratchpadRootURI,
		UIDist:                   c.UIDist,
		Debug:                    c.Debug,
		Policy:                   c.Policy,
		ExposeMCP:                c.ExposeMCP,
	}
}
