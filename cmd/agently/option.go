package agently

// Options is the root command that groups sub-commands.  The struct tags are
// interpreted by github.com/jessevdk/go-flags.
type Options struct {
	Version      bool             `short:"v" long:"version" description:"Show agently version and exit"`
	Serve        *ServeCmd        `command:"serve" description:"Start HTTP server"`
	Scheduler    *SchedulerCmd    `command:"scheduler" description:"Scheduler runner and utilities"`
	Query        *ChatCmd         `command:"query" description:"Query an agent (single turn or continuation)"`
	Chat         *ChatCmd         `command:"chat"  description:"Deprecated alias of query"`
	ListTools    *ListToolsCmd    `command:"list-tools" description:"List available tools"`
	TemplateLoad *TemplateLoadCmd `command:"template-load" description:"Load and validate a template file or workspace template"`
	MCP          *MCPCmd          `command:"mcp" description:"MCP-oriented tool discovery and execution"`
	ChatGPTLogin *ChatGPTLoginCmd `command:"chatgpt-login" description:"Login via ChatGPT OAuth and persist tokens for OpenAI providers"`
}

// Init instantiates the sub-command referenced by the first argument so that
// flags.Parse can populate its fields.
func (o *Options) Init(firstArg string) {
	switch firstArg {
	case "serve":
		o.Serve = &ServeCmd{}
	case "scheduler":
		o.Scheduler = &SchedulerCmd{}
	case "chat":
		o.Chat = &ChatCmd{}
	case "query":
		o.Query = &ChatCmd{}
	case "list-tools":
		o.ListTools = &ListToolsCmd{}
	case "template-load":
		o.TemplateLoad = &TemplateLoadCmd{}
	case "mcp":
		o.MCP = &MCPCmd{}
	case "chatgpt-login":
		o.ChatGPTLogin = &ChatGPTLoginCmd{}
	}
}
