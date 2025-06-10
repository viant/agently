package agently

// CLI helpers for managing configured MCP servers in the executor config.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jessevdk/go-flags"
	"gopkg.in/yaml.v3"

	ex "github.com/viant/agently/genai/executor"
	mcpcfg "github.com/viant/fluxor-mcp/mcp/config"
	"github.com/viant/mcp"
)

// ---------------------------------------------------------------------------
// Root command
// ---------------------------------------------------------------------------

type McpCmd struct {
	Add    *McpAddCmd    `command:"add"    description:"Add a new MCP server to config"`
	Remove *McpRemoveCmd `command:"remove" description:"Remove an MCP server from config by name"`
	List   *McpListCmd   `command:"list"   description:"List configured MCP servers"`
	// Provide default path override – falls back to ./agently/config.yaml
	Config string `short:"f" long:"config" description:"Path to executor config YAML"`
}

func (c *McpCmd) Execute(args []string) error { //nolint:revive – required by go-flags
	// go-flags executes the concrete sub-command, so this is only reached when
	// no sub-command is provided – treat as help.
	return flags.ErrHelp
}

// ---------------------------------------------------------------------------
// Common helpers
// ---------------------------------------------------------------------------

func defaultConfigPath() string {
	if home, _ := os.UserHomeDir(); home != "" {
		return filepath.Join(home, ".agently", "config.yaml")
	}
	// fallback – mostly for tests or unusual environments
	return filepath.FromSlash("./agently/config.yaml")
}

func loadConfig(path string) (*ex.Config, error) {
	cfg := &ex.Config{}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil // return empty/default config
		}
		return nil, err
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func saveConfig(path string, cfg *ex.Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func ensureMcpGroup(cfg *ex.Config) *mcpcfg.Group[*mcp.ClientOptions] {
	if cfg.MCP == nil {
		cfg.MCP = &mcpcfg.Group[*mcp.ClientOptions]{}
	}
	return cfg.MCP
}

// ---------------------------------------------------------------------------
// add
// ---------------------------------------------------------------------------

type McpAddCmd struct {
	Name string `short:"n" long:"name" description:"Identifier for the MCP server" required:"yes"`

	// Transport selection
	Type string `short:"t" long:"type" description:"Transport type: stdio | sse | streaming" choice:"stdio" choice:"sse" choice:"streaming" required:"yes"`

	// HTTP transport
	URL string `long:"url" description:"Server URL for HTTP transport"`

	// Stdio transport
	Command   string   `long:"command" description:"Command path for stdio transport"`
	Arguments []string `long:"arg" description:"Additional command arguments (repeatable)"`

	Config string `short:"f" long:"config" description:"Executor config path (default ./agently/config.yaml)"`
}

func (c *McpAddCmd) Execute(_ []string) error {
	path := c.Config
	if path == "" {
		path = defaultConfigPath()
	}

	cfg, err := loadConfig(path)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	group := ensureMcpGroup(cfg)

	// ------------------------------------------------------------------
	// Build ClientOptions from flags
	// ------------------------------------------------------------------
	opts := &mcp.ClientOptions{Name: c.Name}

	switch strings.ToLower(c.Type) {
	case "stdio":
		if c.Command == "" {
			return fmt.Errorf("--command is required for stdio transport")
		}
		opts.Transport = mcp.ClientTransport{
			Type: "stdio",
			ClientTransportStdio: mcp.ClientTransportStdio{
				Command:   c.Command,
				Arguments: c.Arguments,
			},
		}
	case "sse", "streaming":
		if c.URL == "" {
			return fmt.Errorf("--url is required for HTTP transport")
		}
		opts.Transport = mcp.ClientTransport{
			Type: c.Type,
			ClientTransportHTTP: mcp.ClientTransportHTTP{
				URL: c.URL,
			},
		}
	default:
		return fmt.Errorf("unsupported transport type %q", c.Type)
	}

	// Overwrite existing entry with same name or append new.
	replaced := false
	for idx, item := range group.Items {
		if item != nil && item.Name == c.Name {
			group.Items[idx] = opts
			replaced = true
			break
		}
	}
	if !replaced {
		group.Items = append(group.Items, opts)
	}

	if err := saveConfig(path, cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Printf("MCP server %q added to %s\n", c.Name, path)

	// The running executor instance does not auto-reload; inform user.
	fmt.Println("Restart Agently CLI commands to use the updated configuration.")
	return nil
}

// ---------------------------------------------------------------------------
// remove
// ---------------------------------------------------------------------------

type McpRemoveCmd struct {
	Name   string `short:"n" long:"name" description:"Identifier of MCP server to remove" required:"yes"`
	Config string `short:"f" long:"config" description:"Executor config path (default ./agently/config.yaml)"`
}

func (c *McpRemoveCmd) Execute(_ []string) error {
	path := c.Config
	if path == "" {
		path = defaultConfigPath()
	}

	cfg, err := loadConfig(path)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	group := ensureMcpGroup(cfg)
	if len(group.Items) == 0 {
		return fmt.Errorf("no MCP servers configured")
	}

	var newItems []*mcp.ClientOptions
	removed := false
	for _, item := range group.Items {
		if item != nil && item.Name == c.Name {
			removed = true
			continue
		}
		newItems = append(newItems, item)
	}
	if !removed {
		return fmt.Errorf("MCP server %q not found", c.Name)
	}
	group.Items = newItems

	if err := saveConfig(path, cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Printf("MCP server %q removed from %s\n", c.Name, path)
	return nil
}

// ---------------------------------------------------------------------------
// list
// ---------------------------------------------------------------------------

type McpListCmd struct {
	JSON   bool   `long:"json" description:"Print list as JSON"`
	Config string `short:"f" long:"config" description:"Executor config path (default ./agently/config.yaml)"`
}

func (c *McpListCmd) Execute(_ []string) error {
	path := c.Config
	if path == "" {
		path = defaultConfigPath()
	}

	cfg, err := loadConfig(path)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	group := ensureMcpGroup(cfg)
	if len(group.Items) == 0 {
		fmt.Println("no MCP servers configured")
		return nil
	}

	if c.JSON {
		data, _ := json.MarshalIndent(group.Items, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	for _, item := range group.Items {
		if item == nil {
			continue
		}
		fmt.Printf("%s\t%s\n", item.Name, item.Transport.Type)
	}
	return nil
}
