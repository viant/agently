package agently

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadWindowMigrations(t *testing.T) {
	root := t.TempDir()
	if option, err := loadWindowMigrations(root); err != nil || option != nil {
		t.Fatal("absent manifest changed behavior")
	}
	file := filepath.Join(root, "resource-migrations.json")
	for _, raw := range []string{`{"version":2}`, `{"version":1,"unknown":true}`, `{"version":1} {}`} {
		if err := os.WriteFile(file, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadWindowMigrations(root); err == nil {
			t.Fatalf("invalid manifest accepted: %s", raw)
		}
	}
	raw := `{"version":1,"windows":[{"from":{"providerIdentity":"old","uri":"window://old/records"},"to":{"providerIdentity":"studio","uri":"window://new/records"}}]}`
	if err := os.WriteFile(file, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if option, err := loadWindowMigrations(root); err != nil || option == nil {
		t.Fatalf("valid manifest: %v", err)
	}
}
