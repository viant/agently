package metadata

import (
	"embed"

	_ "github.com/viant/afs/embed"
)

// FS exposes the embedded Forge UI metadata bundled with the app.
//
//go:embed *
var FS embed.FS
