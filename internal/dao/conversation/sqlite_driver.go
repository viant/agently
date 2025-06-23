//go:build cgo

package conversation

// Register the CGO-based SQLite driver when CGO is enabled. Building without
// CGO (e.g. on systems without a C toolchain) drops this file, preventing
// compilation errors stemming from the driver’s C dependencies.

import (
	_ "github.com/mattn/go-sqlite3"
)
