package baserepo

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/afs"
)

func TestRepository_List_FiltersInvalidEntries(t *testing.T) {
	type testCase struct {
		name     string
		setup    func(root string) error
		expected []string
	}

	// Prepare a temporary AGENTLY_ROOT and agents directory content.
	root := t.TempDir()
	_ = os.Setenv("AGENTLY_ROOT", root)
	agentsDir := filepath.Join(root, "agents")
	_ = os.MkdirAll(agentsDir, 0755)

	cases := []testCase{
		{
			name: "mix of valid yaml, non-yaml and nested layout",
			setup: func(root string) error {
				// Flat valid YAML
				if err := os.WriteFile(filepath.Join(root, "agents", "valid.yaml"), []byte("id: valid\n"), 0644); err != nil {
					return err
				}
				// Non-YAML files should be ignored
				if err := os.WriteFile(filepath.Join(root, "agents", "ignore.txt"), []byte("nop"), 0644); err != nil {
					return err
				}
				if err := os.WriteFile(filepath.Join(root, "agents", "ROLES"), []byte("nop"), 0644); err != nil {
					return err
				}
				// Nested layout: <name>/<name>.yaml should be included
				if err := os.MkdirAll(filepath.Join(root, "agents", "nested"), 0755); err != nil {
					return err
				}
				if err := os.WriteFile(filepath.Join(root, "agents", "nested", "nested.yaml"), []byte("id: nested\n"), 0644); err != nil {
					return err
				}
				// Mismatched nested file name should be ignored
				if err := os.MkdirAll(filepath.Join(root, "agents", "mismatch"), 0755); err != nil {
					return err
				}
				if err := os.WriteFile(filepath.Join(root, "agents", "mismatch", "other.yaml"), []byte("id: other\n"), 0644); err != nil {
					return err
				}
				return nil
			},
			expected: []string{"nested", "valid"},
		},
	}

	fs := afs.New()
	repo := New[struct{}](fs, "agents")
	ctx := context.Background()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Clean and re-create agents directory for each case
			_ = os.RemoveAll(agentsDir)
			_ = os.MkdirAll(agentsDir, 0755)

			if err := tc.setup(root); err != nil {
				t.Fatalf("setup failed: %v", err)
			}

			got, err := repo.List(ctx)
			assert.EqualValues(t, nil, err)
			sort.Strings(got)
			sort.Strings(tc.expected)
			assert.EqualValues(t, tc.expected, got)
		})
	}
}
