package router

import (
	"net/http"

	chat "github.com/viant/agently/adapter/http"
	"github.com/viant/agently/adapter/http/workspace"
	execsvc "github.com/viant/agently/genai/executor"
	"github.com/viant/agently/service"
)

// New constructs an http.Handler that combines chat API and workspace CRUD API.
//
// Chat endpoints are mounted under /v1/api/… (see adapter/http/server.go).
// Workspace endpoints under /v1/workspace/… (see adapter/http/workspace).
func New(exec *execsvc.Service, svc *service.Service) http.Handler {
	mux := http.NewServeMux()

	// Chat & workspace endpoints (existing)
	mux.Handle("/v1/api/", chat.NewServer(exec.Conversation()))
	mux.Handle("/v1/workspace/", workspace.NewHandler(svc))

	return chat.WithCORS(mux)
}
