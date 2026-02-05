package extensionrepo

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/afs"
)

func TestRepository_Load_View(t *testing.T) {
	cases := []struct {
		name               string
		file               string
		yaml               string
		expectID           string
		expectContainerIDs []string
	}{
		{
			name: "terminal-single-container",
			file: "terminal.yaml",
			yaml: "id: terminal\n" +
				"title: Terminal\n" +
				"match:\n  service: system/exec\n  method: execute\n" +
				"activation:\n  kind: history\n" +
				"ui:\n  id: commands\n  type: table\n  dataSourceRef: commands\n  columns:\n    - id: input\n      name: Command\n    - id: output\n      name: Stdout\n",
			expectID: "commands",
		},
		{
			name: "plan-multi-container",
			file: "plan.yaml",
			yaml: "id: plan-updates\n" +
				"title: Plan\n" +
				"match:\n  service: orchestration\n  method: updatePlan\n" +
				"activation:\n  kind: history\n  scope: all\n" +
				"ui:\n  containers:\n  - id: header\n    items:\n      - id: explanation\n        type: label\n  - id: planTable\n    type: table\n    columns:\n      - id: status\n        name: ExcludeStatuses\n",
			expectContainerIDs: []string{"header", "planTable"},
		},
	}

	root := t.TempDir()
	_ = os.Setenv("AGENTLY_WORKSPACE", root)
	_ = os.MkdirAll(filepath.Join(root, "feeds"), 0755)

	fs := afs.New()
	repo := New(fs)
	ctx := context.Background()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(root, "feeds", tc.file)
			if err := os.WriteFile(path, []byte(tc.yaml), 0644); err != nil {
				t.Fatal(err)
			}
			rec, err := repo.Load(ctx, strings.TrimSuffix(tc.file, ".yaml"))
			assert.EqualValues(t, nil, err)
			assert.NotNil(t, rec)
			raw, err := json.Marshal(rec.UI)
			assert.EqualValues(t, nil, err)
			var got map[string]interface{}
			_ = json.Unmarshal(raw, &got)
			t.Logf("ui for %s: %#v", tc.name, got)

			if tc.expectID != "" {
				assert.EqualValues(t, tc.expectID, got["id"])
			}
			if len(tc.expectContainerIDs) > 0 {
				rawContainers, ok := got["containers"].([]interface{})
				if assert.True(t, ok, "containers should be a slice") {
					var ids []string
					for _, c := range rawContainers {
						if m, ok := c.(map[string]interface{}); ok {
							if id, _ := m["id"].(string); id != "" {
								ids = append(ids, id)
							}
						}
					}
					assert.EqualValues(t, tc.expectContainerIDs, ids)
				}
			}
		})
	}
}
