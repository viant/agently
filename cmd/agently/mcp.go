package agently

// New implementation uses workspace repository to manage MCP servers.

import (
	"context"
	"fmt"
	"strings"

	"github.com/viant/agently/cmd/service"
	"github.com/viant/mcp"
	"gopkg.in/yaml.v3"
)

// Root command ---------------------------------------------

type McpCmd struct {
	Add    *McpAddCmd    `command:"add" description:"Add or update MCP server"`
	Remove *McpRemoveCmd `command:"remove" description:"Delete MCP server"`
	List   *McpListCmd   `command:"list" description:"List MCP servers"`
}

// ------------------------------------------------------------------ add -----

type McpAddCmd struct {
	Name string `short:"n" long:"name" required:"yes" description:"identifier"`

	// Accept legacy and new aliases: stdio|sse|streaming plus streamable/streamableHTTP
	Type string `short:"t" long:"type" choice:"stdio" choice:"sse" choice:"streaming" choice:"streamable" choice:"streamableHTTP" required:"yes"`

	// HTTP
	URL string `long:"url" description:"server URL when type=sse|streaming"`

	// stdio
	Command   string   `long:"command" description:"command path when type=stdio"`
	Arguments []string `long:"arg" description:"extra arguments (repeatable)"`
}

func (c *McpAddCmd) Execute(_ []string) error {
	opts := &mcp.ClientOptions{Name: c.Name}

	switch strings.ToLower(c.Type) {
	case "stdio":
		if c.Command == "" {
			return fmt.Errorf("--command required for stdio transport")
		}
		opts.Transport = mcp.ClientTransport{
			Type: "stdio",
			ClientTransportStdio: mcp.ClientTransportStdio{
				Command:   c.Command,
				Arguments: c.Arguments,
			},
		}
	case "sse", "streamable":
		// Normalize new aliases to streaming transport
		t := strings.ToLower(c.Type)
		if c.URL == "" {
			return fmt.Errorf("--url required for HTTP transport")
		}
		opts.Transport = mcp.ClientTransport{
			Type:                t,
			ClientTransportHTTP: mcp.ClientTransportHTTP{URL: c.URL},
		}
	default:
		return fmt.Errorf("unsupported type %s", c.Type)
	}

	svc := service.New(executorSingleton(), service.Options{})
	repo := svc.MCPRepo()

	data, _ := yaml.Marshal(opts)
	return repo.Add(context.Background(), c.Name, data)
}

// ------------------------------------------------------------------ remove --

type McpRemoveCmd struct {
	Name string `short:"n" long:"name" required:"yes" description:"identifier to delete"`
}

func (c *McpRemoveCmd) Execute(_ []string) error {
	svc := service.New(executorSingleton(), service.Options{})
	return svc.MCPRepo().Delete(context.Background(), c.Name)
}

// ------------------------------------------------------------------ list ----

type McpListCmd struct {
	JSON bool `long:"json" description:"output full JSON objects"`
}

func (c *McpListCmd) Execute(_ []string) error {
	svc := service.New(executorSingleton(), service.Options{})
	repo := svc.MCPRepo()
	ctx := context.Background()

	names, err := repo.List(ctx)
	if err != nil {
		return err
	}

	if !c.JSON {
		for _, n := range names {
			fmt.Println(n)
		}
		return nil
	}

	var items []*mcp.ClientOptions
	for _, n := range names {
		raw, err := repo.GetRaw(ctx, n)
		if err != nil {
			continue
		}
		var cfg mcp.ClientOptions
		if err := yaml.Unmarshal(raw, &cfg); err == nil {
			items = append(items, &cfg)
		}
	}
	return nil
}
