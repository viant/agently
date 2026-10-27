package ui

import (
	"io/fs"
	"path"
	"regexp"
	"strings"
	"testing"
)

func TestEmbeddedEntryPoints(t *testing.T) {
	for _, name := range []string{"index.html", "mcp-app-sandbox.html", "favicon.ico"} {
		data, err := FS.ReadFile(name)
		if err != nil {
			t.Errorf("missing embedded entry %s: %v", name, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("empty embedded entry %s", name)
		}
	}
}

// Inspect emitted references, including lazy chunks and CSS fonts, rather than
// assuming that a successful frontend build guarantees a complete Go bundle.
func TestEmbeddedAssetReferences(t *testing.T) {
	refs := regexp.MustCompile(`["'(]((?:/?assets/|\./)[^"'()?#\s]+\.(?:js|css|woff2?|ttf|eot|svg|png|ico))["')]`)
	err := fs.WalkDir(FS, ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		switch path.Ext(name) {
		case ".html", ".js", ".css":
		default:
			return nil
		}
		data, err := FS.ReadFile(name)
		if err != nil {
			return err
		}
		for _, match := range refs.FindAllSubmatch(data, -1) {
			ref := string(match[1])
			target := strings.TrimPrefix(ref, "/")
			if strings.HasPrefix(ref, "./") {
				target = path.Join(path.Dir(name), ref)
			}
			if _, err := fs.Stat(FS, target); err != nil {
				t.Errorf("%s references missing embedded asset %s", name, target)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
