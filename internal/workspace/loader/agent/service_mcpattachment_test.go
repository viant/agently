package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoader_ParseMCPResources(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	file := filepath.Join(dir, "agent.yaml")

	yaml := `
agent:
  id: test
  name: test-agent
  mcpResources:
    enabled: true
    maxFiles: 3
    trimPath: "/root/"
    locations:
      - "/root/a.txt"
      - "/root/b.md"
    match:
      inclusions: ["**/*.txt", "**/*.md"]
      exclusions: ["**/*.log"]
      maxFileSize: 1024
`
	if err := os.WriteFile(file, []byte(yaml), 0644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	svc := New()
	got, err := svc.Load(ctx, file)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Basic agent sanity
	assert.EqualValues(t, "test", got.ID)
	assert.NotNil(t, got.MCPResources)
	att := got.MCPResources
	assert.EqualValues(t, true, att.Enabled)
	assert.EqualValues(t, 3, att.MaxFiles)
	assert.EqualValues(t, "/root/", att.TrimPath)
	assert.EqualValues(t, []string{"/root/a.txt", "/root/b.md"}, att.Locations)
	if assert.NotNil(t, att.Match) {
		assert.EqualValues(t, []string{"**/*.txt", "**/*.md"}, att.Match.Inclusions)
		assert.EqualValues(t, []string{"**/*.log"}, att.Match.Exclusions)
		assert.EqualValues(t, 1024, att.Match.MaxFileSize)
	}
}
