package agently

// MCPCmd groups MCP-oriented subcommands.
type MCPCmd struct {
	List *MCPListCmd `command:"list" description:"List tools as MCP-style capabilities"`
	Run  *MCPRunCmd  `command:"run" description:"Run a tool by exact name with JSON arguments"`
}
