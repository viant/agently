package http

import (
	"encoding/json"
	"fmt"
	"github.com/viant/agently/adapter/http/ui"
	"github.com/viant/agently/genai/conversation"
	agentpkg "github.com/viant/agently/genai/extension/fluxor/llm/agent"
	"github.com/viant/agently/metadata"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/tool"
)

// Server wraps a conversation manager and exposes minimal REST endpoints:
//
//	POST /v1/conversations                 -> {"id": "..."}
//	POST /v1/conversations/{id}/messages   -> user message, returns agent reply
//	GET  /v1/conversations/{id}/messages   -> full history (not yet implemented)
//
// The server is designed to be simple and lightweight, suitable for quick
type Server struct {
	mgr        *conversation.Manager
	traceStore *memory.TraceStore
	titles     sync.Map // convID -> title
}

// ServerOption customises HTTP server behaviour.
type ServerOption func(*Server)

// WithTraceStore attaches an in-memory ExecutionTrace store so that GET
// /v1/api/conversations/{id}/tool-trace can return audit information.
func WithTraceStore(ts *memory.TraceStore) ServerOption {
	return func(s *Server) { s.traceStore = ts }
}

// NewServer returns an http.Handler with routes bound.
func NewServer(mgr *conversation.Manager, opts ...ServerOption) http.Handler {
	s := &Server{mgr: mgr}
	for _, o := range opts {
		if o != nil {
			o(s)
		}
	}
	mux := http.NewServeMux()

	// Chat endpoints
	mux.HandleFunc("/v1/api/conversations", s.handleConversations)            // list or create
	mux.HandleFunc("/v1/api/conversations/", s.dispatchConversationSubroutes) // id specific

	// Forge UI metadata endpoints
	// Serve UI metadata from embedded YAML definitions.
	uiRoot := "embed://localhost/"
	uiHandler := ui.NewEmbeddedHandler(uiRoot, &metadata.FS)
	mux.Handle("/v1/api/agently/forge/", http.StripPrefix("/v1/api/agently/forge", uiHandler))

	return WithCORS(mux)
}

// apiResponse is the unified wrapper returned by all Agently HTTP endpoints.
type apiResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// encode writes JSON response with the unified structure.
func encode(w http.ResponseWriter, statusCode int, data interface{}, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		if statusCode == 0 {
			statusCode = http.StatusInternalServerError
		}
		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(apiResponse{Status: "ERROR", Message: err.Error()})
		return
	}
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(apiResponse{Status: "OK", Data: data})
}

// handleConversations supports POST to create new conversation id.
func (s *Server) handleConversations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		id := uuid.NewString()
		title := fmt.Sprintf("Conversation %s", time.Now().Format("2006-01-02 15:04:05"))
		s.titles.Store(id, title)
		encode(w, http.StatusOK, map[string]string{"id": id, "title": title}, nil)
	case http.MethodGet:
		ids, err := s.mgr.List(r.Context())
		if err != nil {
			encode(w, http.StatusInternalServerError, nil, err)
			return
		}
		out := make([]map[string]string, len(ids))
		for i, id := range ids {
			title, _ := s.titles.Load(id)
			var t string
			if titleStr, ok := title.(string); ok {
				t = titleStr
			}
			if t == "" {
				t = id
			}
			out[i] = map[string]string{"id": id, "title": t}
		}
		encode(w, http.StatusOK, out, nil)
	default:
		encode(w, http.StatusMethodNotAllowed, nil, fmt.Errorf("method not allowed"))
	}
}

// handleConversationMessages handles POST /v1/conversations/{id}/messages
func (s *Server) handleConversationMessages(w http.ResponseWriter, r *http.Request, convID string) {
	switch r.Method {
	case http.MethodPost:
		s.handlePostMessage(w, r, convID)
	case http.MethodGet:
		s.handleGetMessages(w, r, convID)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// handleGetMessages supports GET /v1/api/conversations/{id}/messages to return full history.
func (s *Server) handleGetMessages(w http.ResponseWriter, r *http.Request, convID string) {
	msgs, err := s.mgr.Messages(r.Context(), convID)
	if err != nil {
		encode(w, http.StatusInternalServerError, nil, err)
		return
	}
	encode(w, http.StatusOK, msgs, nil)
}

// dispatchConversationSubroutes routes /v1/api/conversations/{id}/... paths to
// dedicated handlers.
func (s *Server) dispatchConversationSubroutes(w http.ResponseWriter, r *http.Request) {
	// Trim prefix and split.
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/api/conversations/"), "/")
	if len(parts) < 1 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	convID := parts[0]
	if convID == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// When only /{id} without trailing path – not supported.
	if len(parts) == 1 {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	switch parts[1] {
	case "messages":
		s.handleConversationMessages(w, r, convID)
	case "tool-trace":
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		s.handleGetToolTrace(w, r, convID)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// handleGetToolTrace responds with the full list of ExecutionTrace entries for the
// given conversation ID.
func (s *Server) handleGetToolTrace(w http.ResponseWriter, r *http.Request, convID string) {
	if s.traceStore == nil {
		// Trace store not hooked up – return empty slice for compatibility.
		encode(w, http.StatusOK, []memory.ExecutionTrace{}, nil)
		return
	}
	traces, err := s.traceStore.List(r.Context(), convID)
	if err != nil {
		encode(w, http.StatusInternalServerError, nil, err)
		return
	}
	encode(w, http.StatusOK, traces, nil)
}

type postMessageRequest struct {
	Content string `json:"content"`
	// Optionally let client point to agent config location
	AgentLocation string `json:"agentLocation,omitempty"`
	ID            string `json:"id,omitempty"`
}

// defaultLocation returns supplied if not empty otherwise "chat".
func defaultLocation(loc string) string {
	if strings.TrimSpace(loc) == "" {
		return "chat"
	}
	return loc
}

func (s *Server) handlePostMessage(w http.ResponseWriter, r *http.Request, convID string) {
	var req postMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		encode(w, http.StatusBadRequest, nil, err)
		return
	}

	input := &agentpkg.QueryInput{
		ConversationID: convID,
		Query:          req.Content,
		Location:       defaultLocation(req.AgentLocation),
		MessageID:      req.ID,
	}

	// Manager/QueryHandler will persist history; no need to duplicate here

	ctx := tool.WithPolicy(r.Context(), &tool.Policy{Mode: tool.ModeAuto})

	_, err := s.mgr.Accept(ctx, input)
	if err != nil {
		encode(w, http.StatusInternalServerError, nil, err)
		return
	}
	// History already updated by downstream services
	// build updated history slice
	msgs, _ := s.mgr.Messages(r.Context(), convID)
	encode(w, http.StatusOK, msgs, nil)
}

// ListenAndServe Simple helper to start the server (blocks).
func ListenAndServe(addr string, mgr *conversation.Manager) error {
	handler := NewServer(mgr)
	log.Printf("HTTP chat server listening on %s", addr)
	return http.ListenAndServe(addr, handler)
}
