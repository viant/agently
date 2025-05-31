package agently

// Options is the root command that groups sub-commands.  The struct tags are
// interpreted by github.com/jessevdk/go-flags.
type Options struct {
    Config string `short:"f" long:"config" description:"executor config YAML/JSON path"`
    Chat  *ChatCmd  `command:"chat"  description:"Chat with an agent (single turn or continuation)"`
    List  *ListCmd  `command:"list"  description:"List existing conversations"`
    Run   *RunCmd   `command:"run"   description:"Run agentic workflow from JSON input"`
    Serve *ServeCmd `command:"serve" description:"Start HTTP server"`
}

// Init instantiates the sub-command referenced by the first argument so that
// flags.Parse can populate its fields.
func (o *Options) Init(firstArg string) {
    switch firstArg {
    case "chat":
        o.Chat = &ChatCmd{}
    case "list":
        o.List = &ListCmd{}
    case "run":
        o.Run = &RunCmd{}
    case "serve":
        o.Serve = &ServeCmd{}
    }
}
