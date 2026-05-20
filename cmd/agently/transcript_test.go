package agently

import "testing"

func TestOptionsInit_Transcript(t *testing.T) {
	opts := &Options{}
	opts.Init("transcript")
	if opts.Transcript == nil {
		t.Fatalf("expected transcript command to initialize")
	}
}

func TestTranscriptCmd_AsChatCarriesAuthFlags(t *testing.T) {
	cmd := &TranscriptCmd{
		API:      "http://127.0.0.1:9191",
		Token:    "token-1",
		OOB:      "~/.secret/demo.enc|blowfish://default",
		OAuthCfg: "~/.secret/idp.enc|blowfish://default",
		OAuthScp: "openid,email",
		User:     "devuser",
	}
	chat := cmd.asChat()
	if chat.API != cmd.API || chat.Token != cmd.Token || chat.OOB != cmd.OOB || chat.OAuthCfg != cmd.OAuthCfg || chat.OAuthScp != cmd.OAuthScp || chat.User != cmd.User {
		t.Fatalf("expected auth/base-url flags to carry over to chat helper")
	}
}
