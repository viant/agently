package agently

// This file wires the stdin-based elicitation awaiter into the executor
// *after* CLI argument parsing so that we know the selected sub-command. This
// replaces the previous init()-time registration which ran *before* main()
// could override os.Args, causing the awaiter to be active even for
// headless/server modes.

import (
	"github.com/viant/agently/genai/executor"
)

// attachAwaiter registers the stdin awaiter unless the first CLI argument is
// "serve" (HTTP mode) where blocking on STDIN must be avoided.
func attachAwaiter(firstArg string) {
	if firstArg == "serve" || firstArg == "scheduler" {
		return // server must never block on stdin
	}
	registerExecOption(executor.WithNewElicitationAwaiter(newStdinAwaiter))
}
