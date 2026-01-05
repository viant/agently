package patch

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestNewSession_UsesValidFileURL(t *testing.T) {
	session, err := NewSession()
	if err != nil {
		t.Fatalf("NewSession() error: %v", err)
	}
	if !strings.HasPrefix(session.tempDir, "file:///") {
		t.Fatalf("expected tempDir to start with file:/// but got %q", session.tempDir)
	}
	if !strings.HasPrefix(filepath.ToSlash(session.tempDir), "file:///") {
		t.Fatalf("expected tempDir to be a valid file URL but got %q", session.tempDir)
	}
}
