package integrate

import (
	"context"
	"os/exec"
	"runtime"
)

// OSBrowserPrompt opens the authorization URL in the system browser.
type OSBrowserPrompt struct{}

func (OSBrowserPrompt) PromptOOB(_ context.Context, authorizationURL string, _ OAuthMeta) error {
	if authorizationURL == "" {
		return nil
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", authorizationURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", authorizationURL)
	default:
		cmd = exec.Command("xdg-open", authorizationURL)
	}
	_ = cmd.Start() // best effort; do not block or propagate error
	return nil
}
