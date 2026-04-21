package agently

import (
	"testing"

	"github.com/jessevdk/go-flags"
)

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
