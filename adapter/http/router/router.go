package router

import (
	"context"
	"net/http"
	"os"

	"github.com/viant/afs"
	fsadapter "github.com/viant/afs/adapter/http"
	chat "github.com/viant/agently/adapter/http"
	"github.com/viant/agently/adapter/http/filebrowser"
	toolhttp "github.com/viant/agently/adapter/http/tool"
	"github.com/viant/agently/adapter/http/workflow"
	"github.com/viant/agently/deployment/ui"
	elicrouter "github.com/viant/agently/genai/elicitation/router"

	"github.com/viant/agently/adapter/http/router/metadata"
	"github.com/viant/agently/adapter/http/workspace"
	"github.com/viant/agently/cmd/service"
	execsvc "github.com/viant/agently/genai/executor"
	"github.com/viant/agently/genai/tool"
	fluxorpol "github.com/viant/fluxor/policy"
	fhandlers "github.com/viant/forge/backend/handlers"
	fservice "github.com/viant/forge/backend/service/file"
)

// New constructs an http.Handler that combines chat API and workspace CRUD API.
//
// Chat endpoints are mounted under /v1/api/… (see adapter/http/server.go).
// Workspace endpoints under /v1/workspace/… (see adapter/http/workspace).
func New(exec *execsvc.Service, svc *service.Service, toolPol *tool.Policy, fluxPol *fluxorpol.Policy, mcpR elicrouter.ElicitationRouter) http.Handler {
	mux := http.NewServeMux()

	// Forge file service singleton (reused for upload handlers and chat service)
	fs := fservice.New(os.TempDir())

	mux.Handle("/v1/api/", chat.NewServer(exec.Conversation(),
		chat.WithPolicies(toolPol, fluxPol),
		chat.WithApprovalService(exec.ApprovalService()),
		chat.WithFileService(fs),
		chat.WithMCPRouter(mcpR),
	))
	mux.Handle("/v1/workspace/", workspace.NewHandler(svc))
	// Backward-compatible alias so callers using /v1/api/workspace/* keep working
	mux.Handle("/v1/api/workspace/", http.StripPrefix("/v1/api/", workspace.NewHandler(svc)))

	// Workflow run endpoint
	mux.Handle("/v1/api/workflow/run", workflow.New(exec, svc))

	// Ad-hoc tool execution
	mux.Handle("/v1/api/tools/", http.StripPrefix("/v1/api/tools", toolhttp.New(svc)))

	// File browser (Forge)
	mux.Handle("/v1/workspace/file-browser/", http.StripPrefix("/v1/workspace/file-browser", filebrowser.New()))

	// Preferred path
	mux.HandleFunc("/v1/workspace/metadata", metadata.NewAgently(exec))

	// Forge file upload/list/download endpoints for chat attachments
	mux.HandleFunc("/upload", fhandlers.UploadHandler(fs))
	fb := fhandlers.NewFileBrowser(fs)
	mux.HandleFunc("/download", fb.DownloadHandler)
	mux.HandleFunc("/list", fb.ListHandler)

	fileSystem := fsadapter.New(afs.New(), "embed://localhost", &ui.FS)
	fileServer := http.FileServer(fileSystem)

	// static content
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html")
			w.Write(ui.Index)
			w.WriteHeader(http.StatusOK)
			return
		}
		fileServer.ServeHTTP(w, r)
	})

	mux.HandleFunc("/.well-known/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	// Kick off background sync that surfaces fluxor approval requests as chat messages
	ctx := context.Background()
	chat.StartApprovalBridge(ctx, exec, exec.Conversation())

	return chat.WithCORS(mux)
}

// newStore removed — chat service now uses conversation client directly
