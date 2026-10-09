package agently

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/viant/agently-core/app/executor"
	"github.com/viant/agently-core/workspace"
)

func TestServeAppliesTrustedAuthorizationBuilderBeforeOpeningListeners(t *testing.T) {
	previous := workspace.Root()
	defer workspace.SetRoot(previous)
	t.Setenv("AGENTLY_AUTHZ_CONFIG", "")
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	marker := errors.New("trusted-authorization-startup-fixture")
	called := false
	err = Serve(ServeOptions{WorkspacePath: t.TempDir(), Addr: occupied.Addr().String(), ConfigureBuilder: func(_ context.Context, builder *executor.Builder) error { called = builder != nil; return marker }})
	if !called || err == nil || !strings.Contains(err.Error(), marker.Error()) {
		t.Fatalf("authorization builder was not applied before server startup: called=%v err=%v", called, err)
	}
}
