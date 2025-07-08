package toolhandler

// HTTP handler that exposes simple execution of an MCP tool definition via
// REST API so that the Forge UI can run ad-hoc actions outside of a Fluxor
// workflow.

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/viant/agently/service"
)

// Handler wraps agently Service to execute tools.
type Handler struct {
	svc *service.Service
}

// New constructs handler bound to service.
func New(svc *service.Service) *Handler { return &Handler{svc: svc} }

type apiResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// ServeHTTP expects pattern /v1/api/tools/{name} with optional query timeout
// It accepts POST only. Body should be JSON object with args.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(apiResponse{Status: "ERROR", Message: "method not allowed"})
		return
	}

	// The router mounts this handler using http.StripPrefix("/v1/api/tools", …)
	// therefore r.URL.Path only contains the tool identifier, possibly
	// prefixed by a leading slash (e.g. "/printer:print"). Treat the entire
	// remaining path – without leading/trailing slashes – as the tool name.
	toolName := strings.Trim(r.URL.Path, "/")
	if toolName == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(apiResponse{Status: "ERROR", Message: "tool name missing"})
		return
	}
	if strings.TrimSpace(toolName) == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(apiResponse{Status: "ERROR", Message: "tool name missing"})
		return
	}

	var args map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&args); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(apiResponse{Status: "ERROR", Message: err.Error()})
		return
	}

	timeout := 0 * time.Second
	if tStr := r.URL.Query().Get("timeout"); tStr != "" {
		if dur, err := time.ParseDuration(tStr); err == nil {
			timeout = dur
		}
	}

	// Execute via core service.
	ctx := r.Context()
	res, err := h.svc.ExecuteTool(ctx, service.ToolRequest{Name: toolName, Args: args, Timeout: timeout})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(apiResponse{Status: "ERROR", Message: err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(apiResponse{Status: "ok", Data: res})
}

// Ensure interface implementation
var _ http.Handler = (*Handler)(nil)
