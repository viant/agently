package exec

import (
	"strings"

	sys "github.com/viant/agently/genai/tool/service/system"
)

// Input represents system executor configuration
type Input struct {
	Host         *sys.Host         `json:"host,omitempty" description:"host to execute command on"  internal:"true" `        //host to execute command on
	Workdir      string            `json:"workdir,omitempty"  description:"directory where file system command start"`       //directory where command should run  - if does not exists there is no exception
	Env          map[string]string `json:"env,omitempty" description:"environment variables to be set before command runs" ` //environment variables to be set before command runs
	Commands     []string          `json:"commands,omitempty" description:"commands to execute on the target system"`        //commands to run
	TimeoutMs    int               `json:"timeoutMs,omitempty" yaml:"timeoutMs,omitempty" description:"max wiat time before timing out command"`
	AbortOnError *bool             `json:"abortOnError,omitempty" description:"check after command execution if status is <> 0, then throws error" ` //whether to abort on error
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
