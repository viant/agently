package metadata

import (
	"embed"
	_ "github.com/viant/afs/embed"
)

// FS exposes the embedded Forge UI metadata (navigation & windows).
//
// Embed all metadata files recursively (navigation.yaml, windows, JS, etc.).
// The pattern `*` includes current directory and all sub-directories.
// This guarantees that WindowHandler can locate window definitions such as
// `window/chat/main.yaml` while NavigationHandler can still access
// top-level files like `navigation.yaml`.
//
//go:embed *
var FS embed.FS
