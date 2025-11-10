package exec

import (
	"strings"

	sys "github.com/viant/agently/genai/tool/service/system"
)

// Input represents system executor configuration
type Input struct {
	Host         *sys.Host         `json:"host,omitempty" internal:"true" description:"Target host. Use bash://localhost/ (default) or ssh://user@host:22."`
	Workdir      string            `json:"workdir,omitempty" description:"Working directory for file operations. Example: /repo/path."`
	Env          map[string]string `json:"env,omitempty" description:"Environment variables (k=v) set before running. Example: {'GOFLAGS':'-mod=mod'}."`
	Commands     []string          `json:"commands,omitempty" description:"Commands to run in order (no pipes). Example: ['rg --files', 'sed -n 1,20p file.go']."`
	TimeoutMs    int               `json:"timeoutMs,omitempty" yaml:"timeoutMs,omitempty" description:"Per-command timeout in ms (default 180000)."`
	AbortOnError *bool             `json:"abortOnError,omitempty" description:"Stop on first non-zero status (default true)."`
}

var fsCommands = []string{
	"ls", "cat", "touch", "rm", "mv", "cp", "mkdir", "rmdir",
	"find", "chmod", "chown", "stat", "du", "df",
	"head", "tail", "more", "less",
	"readlink", "ln", "rg", "sed",
	"tree", "basename", "dirname",
	"os.", // for Go/python-like code using os package
}

func (i *Input) HasFSCommand() string {
	for _, cmd := range i.Commands {
		for _, fsCmd := range fsCommands {
			if strings.Contains(cmd, fsCmd) {
				return cmd // return the first filesystem-related command found
			}
		}
	}
	return ""
}

func (i *Input) Init() {
	if i.Host == nil {
		i.Host = &sys.Host{}
	}
	if i.Host.URL == "" {
		i.Host.URL = "bash://localhost/"
	}
}
