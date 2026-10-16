package agently

import (
	"context"
	"encoding/json"
	"io/fs"
	"path"
	"sort"
	"strings"

	identity "github.com/viant/agently-core/protocol/resource"
	resources "github.com/viant/agently-core/service/resource"
	windowloader "github.com/viant/agently-core/service/ui/window"
	"github.com/viant/agently/metadata"
	"github.com/viant/forge/backend/types"
)

// defaultFrameworkWindows declares only linked window entry points. It supplies
// the same trusted loader already used by explicit framework registrations.
func defaultFrameworkWindows() ([]resources.NativeFrameworkWindowBinding, error) {
	keys := map[string]bool{}
	err := fs.WalkDir(metadata.FS, "window", func(file string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() {
			return nil
		}
		if path.Base(file) != "main.yaml" {
			return nil
		}
		parts := strings.Split(strings.TrimPrefix(path.Dir(file), "window/"), "/")
		for i, p := range parts {
			switch p {
			case "shared", "web", "mobile", "android", "ios", "desktop", "phone", "tablet":
				parts = parts[:i]
				goto done
			}
		}
	done:
		key := strings.Join(parts, "/")
		if key != "" {
			keys[key] = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(keys))
	for key := range keys {
		names = append(names, key)
	}
	sort.Strings(names)
	bindings := make([]windowloader.ResourceBinding, 0, len(names))
	for _, key := range names {
		bindings = append(bindings, windowloader.ResourceBinding{WindowKey: key, URI: "window://system/" + key})
	}
	linked, err := newEmbeddedWindowSource(bindings)
	if err != nil {
		return nil, err
	}
	out := make([]resources.NativeFrameworkWindowBinding, 0, len(bindings))
	for _, binding := range bindings {
		binding := binding
		uri, _ := identity.ParseResourceURI(binding.URI)
		out = append(out, resources.NativeFrameworkWindowBinding{Resource: binding, Load: serializedWindowBase(func(ctx context.Context, target *types.WindowTarget) (json.RawMessage, error) {
			return linked.loadTarget(ctx, uri, target)
		})})
	}
	return out, nil
}
