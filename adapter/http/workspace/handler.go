package workspace

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/viant/agently/internal/workspace"
	"github.com/viant/agently/service"
)

// Handler exposes CRUD operations for any workspace repository kind
// (agents, models, workflows, mcp …).
//
// Routes (all under /v1/workspace/):
//
//	GET    /{kind}            -> JSON []string
//	GET    /{kind}/{name}     -> raw YAML/JSON (as stored)
//	PUT    /{kind}/{name}     -> create/overwrite
//	DELETE /{kind}/{name}     -> delete
//
// It is intentionally stateless; repositories are resolved via service.Service.
func NewHandler(svc *service.Service) http.Handler {
	return &handler{svc: svc}
}

type handler struct {
	svc *service.Service
}

// ServeHTTP implements http.Handler.
func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Remove leading slash and known prefix.
	path := strings.TrimPrefix(r.URL.Path, "/")
	if strings.HasPrefix(path, "v1/workspace/") {
		path = strings.TrimPrefix(path, "v1/workspace/")
	}
	parts := strings.Split(path, "/")

	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}

	kind := parts[0]
	repo, ok := h.repo(kind)
	if !ok {
		http.NotFound(w, r)
		return
	}

	ctx := r.Context()

	// List
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		items, err := repo.List(ctx)
		if err != nil {
			writeErr(w, err)
			return
		}
		_ = jsonEncode(w, items)
		return
	}

	// Single item operations: allow names with slashes after {kind}
	name := strings.Join(parts[1:], "/")

	switch r.Method {
	case http.MethodGet:
		data, err := repo.GetRaw(ctx, name)
		if err != nil {
			writeErr(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/x-yaml")
		_, _ = w.Write(data)
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(err.Error()))
			return
		}
		if err := repo.Add(ctx, name, body); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusOK)
	case http.MethodDelete:
		if err := repo.Delete(ctx, name); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusOK)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// repo resolves kind → repository.
func (h *handler) repo(kind string) (rawRepository, bool) {
	switch kind {
	case workspace.KindAgent, "agent":
		return h.svc.AgentRepo(), true
	case workspace.KindModel, "model":
		return h.svc.ModelRepo(), true
	case workspace.KindWorkflow, "workflow":
		return h.svc.WorkflowRepo(), true
	case workspace.KindMCP:
		return h.svc.MCPRepo(), true
	default:
		return nil, false
	}
}

// rawRepository is the minimal set of operations the HTTP layer requires.
type rawRepository interface {
	List(ctx context.Context) ([]string, error)
	GetRaw(ctx context.Context, name string) ([]byte, error)
	Add(ctx context.Context, name string, data []byte) error
	Delete(ctx context.Context, name string) error
}

func jsonEncode(w http.ResponseWriter, v any) error {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("json encode error: %v", err)
		return err
	}
	return nil
}

func writeErr(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = w.Write([]byte(err.Error()))
}
