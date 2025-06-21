package router

import (
	"context"
	"net/http"

	chat "github.com/viant/agently/adapter/http"
	"github.com/viant/agently/adapter/http/workspace"
	execsvc "github.com/viant/agently/genai/executor"
	"github.com/viant/agently/genai/tool"
	"github.com/viant/agently/service"
	fluxorpol "github.com/viant/fluxor/policy"
)

// New constructs an http.Handler that combines chat API and workspace CRUD API.
//
// Chat endpoints are mounted under /v1/api/… (see adapter/http/server.go).
// Workspace endpoints under /v1/workspace/… (see adapter/http/workspace).
func New(exec *execsvc.Service, svc *service.Service, toolPol *tool.Policy, fluxPol *fluxorpol.Policy) http.Handler {
	mux := http.NewServeMux()

	// Chat & workspace endpoints (existing)
	// Default policy inherited from environment variable AGENTLY_POLICY or
	// default to auto. The Serve command will provide an explicit policy via context.
	mux.Handle("/v1/api/", chat.NewServer(exec.Conversation(),
		chat.WithExecutionStore(exec.ExecutionStore()),
		chat.WithPolicies(toolPol, fluxPol)))
	mux.Handle("/v1/workspace/", workspace.NewHandler(svc))

	// Kick off background sync that surfaces fluxor approval requests as chat
	// messages so that web users can approve/reject tool executions.
	ctx := context.Background()
	chat.StartApprovalBridge(ctx, exec, exec.Conversation())

	return chat.WithCORS(mux)
}
