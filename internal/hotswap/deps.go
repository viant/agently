//go:build !test

// Package hotswap aggregates imports that are required by the hot-swap
// feature but are not yet referenced in the early scaffolding stages. The
// blank import ensures the module dependency is added to go.mod so that later
// stages can rely on it without running `go get` manually.
package hotswap

import (
	_ "github.com/fsnotify/fsnotify"
)
