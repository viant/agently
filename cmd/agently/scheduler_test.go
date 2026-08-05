package agently

import (
	"testing"
	"time"

	"github.com/jessevdk/go-flags"
)

func TestSchedulerRunCmd_ScratchpadRootURI(t *testing.T) {
	for _, flag := range []string{"-s", "--scratchpad-root-uri"} {
		t.Run(flag, func(t *testing.T) {
			cmd := &SchedulerRunCmd{}
			parser := flags.NewParser(cmd, flags.HelpFlag|flags.PassDoubleDash)

			_, err := parser.ParseArgs([]string{
				flag, "gs://scratchpad-bucket/users/${userID}",
			})
			if err != nil {
				t.Fatalf("parse args: %v", err)
			}

			options, err := cmd.schedulerRunOptions()
			if err != nil {
				t.Fatalf("build scheduler options: %v", err)
			}
			if got, want := options.ScratchpadRootURI, "gs://scratchpad-bucket/users/${userID}"; got != want {
				t.Fatalf("ScratchpadRootURI = %q, want %q", got, want)
			}
			if got, want := options.Interval, 30*time.Second; got != want {
				t.Fatalf("Interval = %s, want %s", got, want)
			}
		})
	}
}

func TestOptionsInitScheduler(t *testing.T) {
	opts := &Options{}
	opts.Init("scheduler")
	if opts.Scheduler == nil {
		t.Fatalf("expected scheduler command to be initialized")
	}
}

func TestMCPListCmd_ParsesOOBFlags(t *testing.T) {
	cmd := &MCPListCmd{}
	parser := flags.NewParser(cmd, flags.HelpFlag|flags.PassDoubleDash)
	_, err := parser.ParseArgs([]string{
		"--oob", "~/.secret/demo.enc|blowfish://default",
		"--oauth-config", "scy://oauth/config",
		"--oauth-scopes", "openid,email",
	})
	if err != nil {
		t.Fatalf("parse args: %v", err)
	}
	if cmd.OOB != "~/.secret/demo.enc|blowfish://default" {
		t.Fatalf("expected oob flag to parse, got %q", cmd.OOB)
	}
	if cmd.OAuthCfg != "scy://oauth/config" {
		t.Fatalf("expected oauth-config flag to parse, got %q", cmd.OAuthCfg)
	}
	if cmd.OAuthScp != "openid,email" {
		t.Fatalf("expected oauth-scopes flag to parse, got %q", cmd.OAuthScp)
	}
}
