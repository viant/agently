package agently

import (
	"context"
	"fmt"
	"github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/io/elicitation"
	"github.com/viant/agently/genai/io/elicitation/stdio"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// stdinAwaiter prompts the user on stdin/stdout whenever the runtime requests
// a payload that must satisfy the supplied JSON-schema. The implementation
// delegates to stdio.Prompt so the prompting logic remains in a single unit-
// tested place.
type stdinAwaiter struct{}

// openedURLs remembers which URLs were already attempted to be opened in the
// current process so we do not spam the user with multiple browser tabs when
// the same elicitation repeats (e.g. due to planning retries).
var openedURLs = map[string]struct{}{}

// AwaitElicitation implements elicitation.Awaiter.
func (a *stdinAwaiter) AwaitElicitation(ctx context.Context, req *plan.Elicitation) (*plan.ElicitResult, error) {
	// When the request contains an external URL we first inform the user and
	// optionally attempt to open it with the system browser. The actual payload
	// still has to be entered/pasted afterwards.
	if req != nil {
		url := strings.TrimSpace(req.Url)
		if url != "" {
			// Inform the user once per unique URL.
			if _, done := openedURLs[url]; !done {
				openedURLs[url] = struct{}{}
				fmt.Fprintf(os.Stdout, "\nThe workflow requests additional information. Please follow the link below, complete the form and paste the resulting JSON here.\nURL: %s\n", url)
				// Best-effort attempt to open the browser (non-blocking).
				_ = launchBrowser(url)
			} else {
				// Still remind the user of the URL but skip auto-open.
				fmt.Fprintf(os.Stdout, "\nPlease provide the JSON from previously opened URL: %s\n", url)
			}
		}
	}
	var w io.Writer = os.Stdout // ensure stdout flushing
	return stdio.Prompt(ctx, w, os.Stdin, req)
}

// newStdinAwaiter is a helper for neat option wiring.
func newStdinAwaiter() elicitation.Awaiter { return &stdinAwaiter{} }

// launchBrowser attempts to open the supplied URL using the platform default
// command. It returns error only when the OS command execution itself fails –
// callers are expected to ignore the error as this is a best-effort helper.
func launchBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default: // linux, freebsd, …
		cmd = exec.Command("xdg-open", url)
	}
	if cmd == nil {
		return fmt.Errorf("unsupported OS")
	}
	return cmd.Start() // do not wait
}

// Compile-time assertion.
var _ elicitation.Awaiter = (*stdinAwaiter)(nil)
