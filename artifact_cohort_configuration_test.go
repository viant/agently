package agently

import (
	wscfg "github.com/viant/agently-core/workspace/config"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceCohortConfiguration(t *testing.T) {
	root := t.TempDir()
	relative := "cohorts.yaml"
	path := filepath.Join(root, relative)
	if err := os.WriteFile(path, []byte("namespace: example\n"), 0600); err != nil {
		t.Fatal(err)
	}
	config := &wscfg.Root{Raw: map[string]any{"artifactCohortConfig": relative}}
	got, err := resolveArtifactCohortConfig(root, "", "", config)
	if err != nil || got != path {
		t.Fatalf("workspace config: %q %v", got, err)
	}
	for _, item := range []struct{ explicit, environment, want string }{{"/operator/config", "/environment/config", "/operator/config"}, {"", "/environment/config", "/environment/config"}} {
		got, err = resolveArtifactCohortConfig(root, item.explicit, item.environment, config)
		if err != nil || got != item.want {
			t.Fatalf("precedence: %q %v", got, err)
		}
	}
	for _, bad := range []any{"../outside.yaml", 42, ""} {
		config.Raw["artifactCohortConfig"] = bad
		if _, err = resolveArtifactCohortConfig(root, "", "", config); err == nil {
			t.Fatalf("invalid configuration accepted: %v", bad)
		}
	}
}
