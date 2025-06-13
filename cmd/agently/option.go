package agently

// Options is the root command that groups sub-commands.  The struct tags are
// interpreted by github.com/jessevdk/go-flags.
type Options struct {
	Config    string        `short:"f" long:"config" description:"executor config YAML/JSON path"`
	Chat      *ChatCmd      `command:"chat"  description:"Chat with an agent (single turn or continuation)"`
	List      *ListCmd      `command:"list"  description:"List existing conversations"`
	ListTools *ListToolsCmd `command:"list-tools" description:"List available tools"`
	Exec      *ExecCmd      `command:"exec" description:"Execute a tool"`
	Run       *RunCmd       `command:"run"   description:"Run agentic workflow from JSON input"`
	Workflow  *WorkflowCmd  `command:"workflow" description:"Execute a Fluxor workflow graph"`
	Serve     *ServeCmd     `command:"serve" description:"Start HTTP server"`
	MCP       *McpCmd       `command:"mcp" description:"Manage MCP servers"`
}

// Init instantiates the sub-command referenced by the first argument so that
// flags.Parse can populate its fields.
func (o *Options) Init(firstArg string) {
	switch firstArg {
	case "chat":
		o.Chat = &ChatCmd{}
	case "list":
		o.List = &ListCmd{}
	case "list-tools":
		o.ListTools = &ListToolsCmd{}
	case "exec":
		o.Exec = &ExecCmd{}
	case "run":
		o.Run = &RunCmd{}
	case "workflow":
		o.Workflow = &WorkflowCmd{}
	case "serve":
		o.Serve = &ServeCmd{}
	case "mcp":
		o.MCP = &McpCmd{}
	}
}
