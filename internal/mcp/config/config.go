package config

import (
	mcp "github.com/viant/mcp"
)

// Group is a simple list wrapper used by config with an optional URL root.
type Group[T any] struct {
	URL   string `yaml:"url,omitempty" json:"url,omitempty"`
	Items []T    `yaml:"items,omitempty" json:"items,omitempty"`
}

// MCPClient augments mcp.ClientOptions with optional discovery descriptions and metadata.
type MCPClient struct {
	*mcp.ClientOptions `yaml:",inline" json:",inline"`
	Descriptions       map[string]string      `yaml:"descriptions,omitempty" json:"descriptions,omitempty"`
	Metadata           map[string]interface{} `yaml:"metadata,omitempty" json:"metadata,omitempty"`
	// ToolTimeoutSec overrides the default tool execution timeout when invoking
	// tools on this MCP server. When zero, a system default applies.
	ToolTimeoutSec int `yaml:"toolTimeoutSec,omitempty" json:"toolTimeoutSec,omitempty"`
}
