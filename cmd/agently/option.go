package agently

// Options is the root command that groups sub-commands.  The struct tags are
// interpreted by github.com/jessevdk/go-flags.
type Options struct {
	Version      bool             `short:"v" long:"version" description:"Show agently version and exit"`
	Serve        *ServeCmd        `command:"serve" description:"Start HTTP server"`
	Query        *ChatCmd         `command:"query" description:"Query an agent (single turn or continuation)"`
	Chat         *ChatCmd         `command:"chat"  description:"Deprecated alias of query"`
	ListTools    *ListToolsCmd    `command:"list-tools" description:"List available tools"`
	ChatGPTLogin *ChatGPTLoginCmd `command:"chatgpt-login" description:"Login via ChatGPT OAuth and persist tokens for OpenAI providers"`
}

// Init instantiates the sub-command referenced by the first argument so that
// flags.Parse can populate its fields.
func (o *Options) Init(firstArg string) {
	switch firstArg {
	case "serve":
		o.Serve = &ServeCmd{}
	case "chat":
		o.Chat = &ChatCmd{}
	case "query":
		o.Query = &ChatCmd{}
	case "list-tools":
		o.ListTools = &ListToolsCmd{}
	case "chatgpt-login":
		o.ChatGPTLogin = &ChatGPTLoginCmd{}
	}
}
