package agently

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestInlineLocalScyResource_LocalFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.enc")
	payload := []byte("encrypted-content")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatalf("write temp secret: %v", err)
	}

	got, err := inlineLocalScyResource(path + "|blowfish://default")
	if err != nil {
		t.Fatalf("inlineLocalScyResource() error: %v", err)
	}

	want := "inlined://base64/" + base64.StdEncoding.EncodeToString(payload) + "|blowfish://default"
	if got != want {
		t.Fatalf("inlineLocalScyResource() = %q, want %q", got, want)
	}
}

func TestInlineLocalScyResource_RemoteUnchanged(t *testing.T) {
	input := "gcp://secretmanager/projects/acme/secrets/demo|blowfish://default"
	got, err := inlineLocalScyResource(input)
	if err != nil {
		t.Fatalf("inlineLocalScyResource() error: %v", err)
	}
	if got != input {
		t.Fatalf("inlineLocalScyResource() = %q, want %q", got, input)
	}
}

func TestInlineLocalScyResource_InlineUnchanged(t *testing.T) {
	input := "inlined://base64/QUJD|blowfish://default"
	got, err := inlineLocalScyResource(input)
	if err != nil {
		t.Fatalf("inlineLocalScyResource() error: %v", err)
	}
	if got != input {
		t.Fatalf("inlineLocalScyResource() = %q, want %q", got, input)
	}
}
