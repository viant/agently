package agently

import (
	"bufio"
	"context"
	"fmt"
	"io"
	urlpkg "net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/elicitation"
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
			// Out-of-band interaction: present domain only, offer to open full URL; resolution happens via UI callback
			if _, done := openedURLs[url]; !done {
				openedURLs[url] = struct{}{}
			}
			present := url
			if u, err := urlpkg.Parse(url); err == nil && u.Host != "" {
				present = u.Host
			}
			fmt.Fprintf(os.Stdout, "\nSecure Flow Required.\nOpen: %s\n", present)

			reader := bufio.NewReader(os.Stdin)
			for {
				fmt.Fprint(os.Stdout, "Open URL or decline? [o]pen, [d]ecline (default: o): ")
				line, _ := reader.ReadString('\n')
				sel := strings.ToLower(strings.TrimSpace(line))
				if sel == "" || sel == "o" || sel == "open" {
					_ = launchBrowser(url)
					// Do not resolve here; UI callback will resolve elicitation
					return &plan.ElicitResult{Action: plan.ElicitResultActionAccept}, nil
				}
				if sel == "d" || sel == "decline" || sel == "r" || sel == "reject" {
					// Ask for an optional decline reason to help the agent react appropriately.
					fmt.Fprint(os.Stdout, "Enter decline reason (optional): ")
					reason, _ := reader.ReadString('\n')
					return &plan.ElicitResult{Action: plan.ElicitResultActionDecline, Reason: strings.TrimSpace(reason)}, nil
				}
				fmt.Fprintln(os.Stdout, "Invalid choice. Please enter o or d.")
			}
		}
	}
	var w io.Writer = os.Stdout // ensure stdout flushing
	res, err := elicitation.Prompt(ctx, w, os.Stdin, req)
	if err != nil {
		return nil, err
	}
	// Confirm submission after fields are collected.
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Fprint(os.Stdout, "Submit collected details? [a]ccept, [r]eject (default: a): ")
		line, _ := reader.ReadString('\n')
		sel := strings.ToLower(strings.TrimSpace(line))
		if sel == "" || sel == "a" || sel == "accept" {
			return res, nil
		}
		if sel == "r" || sel == "reject" || sel == "decline" {
			fmt.Fprint(os.Stdout, "Enter decline reason (optional): ")
			reason, _ := reader.ReadString('\n')
			return &plan.ElicitResult{Action: plan.ElicitResultActionDecline, Reason: strings.TrimSpace(reason)}, nil
		}
		fmt.Fprintln(os.Stdout, "Invalid choice. Please enter a or r.")
	}
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
