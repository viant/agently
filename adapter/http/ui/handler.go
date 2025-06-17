package ui

import (
	"embed"
	"net/http"
	"strings"

	"github.com/viant/afs"
	"github.com/viant/afs/url"
	forgeHandlers "github.com/viant/forge/backend/handlers"
	fileSvc "github.com/viant/forge/backend/service/file"
	metaSvc "github.com/viant/forge/backend/service/meta"
)

// NewEmbeddedHandler builds a UI http.Handler backed by an embedded filesystem.
// root should use the "embed:///" scheme (e.g. "embed:///metadata").
func NewEmbeddedHandler(root string, efs *embed.FS) http.Handler {
	return newHandler(root, efs)
}

func newHandler(root string, efs *embed.FS) http.Handler {
	// File service (directory listing) – pass embed FS when provided so that list works for navigation import paths.
	var fsvc *fileSvc.Service

	if efs == nil {
		fsvc = fileSvc.New(root)
	} else {
		fsvc = fileSvc.New(root, efs)
	}

	// Forge UI routes
	mux := http.NewServeMux()

	// -----------------
	// Navigation
	// -----------------
	mux.HandleFunc("/navigation", forgeHandlers.NavigationHandler(fsvc, root))

	// -----------------
	// Windows
	// -----------------
	// Window definitions are stored under the `window/` sub-directory of the root.
	// Use a separate meta service with the adjusted baseURL so that
	//   /window/{key}[/{subKey}]         -> {root}/window/{key}/{subKey or main}/main.yaml
	// resolves correctly.
	windowBase := "/window/"
	windowRoot := root
	if !strings.HasSuffix(windowRoot, "/") {
		windowRoot += "/"
	}
	windowRoot = url.Join(windowRoot, "window")
	var windowMSvc *metaSvc.Service
	if efs == nil {
		windowMSvc = metaSvc.New(afs.New(), windowRoot)
	} else {
		windowMSvc = metaSvc.New(afs.New(), windowRoot, efs)
	}
	mux.Handle(windowBase, forgeHandlers.WindowHandler(windowMSvc, windowRoot, windowBase))

	return mux
}
