package agently

// Options is the root command that groups sub-commands.  The struct tags are
// interpreted by github.com/jessevdk/go-flags.
type Options struct {
	Version      bool             `short:"v" long:"version" description:"Show agently version and exit"`
	Config       string           `short:"f" long:"config" description:"executor config YAML/JSON path"`
	Chat         *ChatCmd         `command:"chat"  description:"Chat with an agent (single turn or continuation)"`
	List         *ListCmd         `command:"list"  description:"List existing conversations"`
	ListTools    *ListToolsCmd    `command:"list-tools" description:"List available tools"`
	Exec         *ExecCmd         `command:"exec" description:"Execute a tool"`
	Run          *RunCmd          `command:"run"   description:"Run agentic workflow from JSON input"`
	ModelSwitch  *ModelSwitchCmd  `command:"model-switch" description:"Switch agent default model"`
	ModelReset   *ModelResetCmd   `command:"model-reset" description:"Clear agent model override"`
	Workspace    *WorkspaceCmd    `command:"ws" description:"Workspace CRUD operations"`
	Serve        *ServeCmd        `command:"serve" description:"StartedAt HTTP server"`
	MCP          *McpCmd          `command:"mcp" description:"Manage MCP servers"`
	ChatGPTLogin *ChatGPTLoginCmd `command:"chatgpt-login" description:"Login via ChatGPT OAuth and persist tokens for OpenAI providers"`
}

// Init instantiates the sub-command referenced by the first argument so that
// flags.Parse can populate its fields.
func (o *Options) Init(firstArg string) {
	// Decide whether the CLI session should attach the interactive stdin
	// awaiter. We do this before executor initialisation so that the option is
	// in effect when the singleton is created later.
	attachAwaiter(firstArg)

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
	case "model-switch":
		o.ModelSwitch = &ModelSwitchCmd{}
	case "model-reset":
		o.ModelReset = &ModelResetCmd{}
	case "ws":
		o.Workspace = &WorkspaceCmd{}
	case "serve":
		o.Serve = &ServeCmd{}
	case "mcp":
		o.MCP = &McpCmd{}
	case "chatgpt-login":
		o.ChatGPTLogin = &ChatGPTLoginCmd{}
	}
}
