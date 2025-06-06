package mcp

import (
	"os/exec"
	"runtime"
)

// defaultOpenURL tries best-effort to open the supplied URL in user browser.
func defaultOpenURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default: // linux, freebsd
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
