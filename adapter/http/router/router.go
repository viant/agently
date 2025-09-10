package router

import (
	"context"
	"net/http"

	"github.com/viant/afs"
	fsadapter "github.com/viant/afs/adapter/http"
	"github.com/viant/agently/deployment/ui"

	chat "github.com/viant/agently/adapter/http"
	"github.com/viant/agently/adapter/http/filebrowser"
	toolhttp "github.com/viant/agently/adapter/http/tool"
	"github.com/viant/agently/adapter/http/workflow"

	"os"
	"strings"

	"github.com/viant/agently/adapter/http/workspace"
	execsvc "github.com/viant/agently/genai/executor"
	"github.com/viant/agently/genai/tool"
	daofactory "github.com/viant/agently/internal/dao/factory"
	d "github.com/viant/agently/internal/domain"
	dstore "github.com/viant/agently/internal/domain/adapter"
	"github.com/viant/agently/service"
	"github.com/viant/datly"
	"github.com/viant/datly/view"
	fluxorpol "github.com/viant/fluxor/policy"
)

// New constructs an http.Handler that combines chat API and workspace CRUD API.
//
// Chat endpoints are mounted under /v1/api/… (see adapter/http/server.go).
// Workspace endpoints under /v1/workspace/… (see adapter/http/workspace).
func New(exec *execsvc.Service, svc *service.Service, toolPol *tool.Policy, fluxPol *fluxorpol.Policy) http.Handler {
	mux := http.NewServeMux()

	store := newStore(context.Background())
	mux.Handle("/v1/api/", chat.NewServer(exec.Conversation(),
		chat.WithPolicies(toolPol, fluxPol),
		chat.WithApprovalService(exec.ApprovalService()),
		chat.WithDomainStore(store),
	))
	mux.Handle("/v1/workspace/", workspace.NewHandler(svc))

	// Workflow run endpoint
	mux.Handle("/v1/api/workflow/run", workflow.New(exec, svc))

	// Ad-hoc tool execution
	mux.Handle("/v1/api/tools/", http.StripPrefix("/v1/api/tools", toolhttp.New(svc)))

	// File browser (Forge)
	mux.Handle("/v1/workspace/file-browser/", http.StripPrefix("/v1/workspace/file-browser", filebrowser.New()))

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

	// Kick off background sync that surfaces fluxor approval requests as chat
	// messages so that web users can approve/reject tool executions.
	ctx := context.Background()
	chat.StartApprovalBridge(ctx, exec, exec.Conversation())

	return chat.WithCORS(mux)
}

func newStore(ctx context.Context) d.Store {
	driver := strings.TrimSpace(os.Getenv("AGENTLY_DB_DRIVER"))
	dsn := strings.TrimSpace(os.Getenv("AGENTLY_DB_DSN"))
	if driver != "" && dsn != "" {
		if dao, err := datly.New(ctx); err == nil {
			_ = dao.AddConnectors(ctx, view.NewConnector("agently", driver, dsn))
			if apis, _ := daofactory.New(ctx, daofactory.DAOSQL, dao); apis != nil {
				store := dstore.New(apis.Conversation, apis.Message, apis.Turn, apis.ModelCall, apis.ToolCall, apis.Payload, apis.Usage)
				return store
			}
		}
	}
	if apis, _ := daofactory.New(ctx, daofactory.DAOInMemory, nil); apis != nil {
		store := dstore.New(apis.Conversation, apis.Message, apis.Turn, apis.ModelCall, apis.ToolCall, apis.Payload, apis.Usage)
		return store
	}
	return nil
}
