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
		name   string
		file   string
		yaml   string
		expect map[string]interface{}
	}{
		{
			name: "terminal-single-container",
			file: "terminal.yaml",
			yaml: "id: terminal\n" +
				"title: Terminal\n" +
				"match:\n  service: system/exec\n  method: execute\n" +
				"activation:\n  kind: history\n" +
				"view:\n  id: commands\n  type: table\n  dataSourceRef: commands\n  columns:\n    - id: input\n      name: Command\n    - id: output\n      name: Stdout\n",
			expect: map[string]interface{}{"id": "commands", "type": "table"},
		},
		{
			name: "plan-multi-container",
			file: "plan.yaml",
			yaml: "id: plan-updates\n" +
				"title: Plan\n" +
				"match:\n  service: orchestration\n  method: updatePlan\n" +
				"activation:\n  kind: history\n  scope: all\n" +
				"view:\n  containers:\n  - id: header\n    items:\n      - id: explanation\n        type: label\n  - id: planTable\n    type: table\n    columns:\n      - id: status\n        name: Status\n",
			expect: map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{"id": "header"},
					map[string]interface{}{"id": "planTable"},
				},
			},
		},
	}

	root := t.TempDir()
	_ = os.Setenv("AGENTLY_ROOT", root)
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
			raw, err := json.Marshal(rec.View)
			assert.EqualValues(t, nil, err)
			var got map[string]interface{}
			_ = json.Unmarshal(raw, &got)
			for k, v := range tc.expect {
				assert.EqualValues(t, v, got[k])
			}
		})
	}
}
