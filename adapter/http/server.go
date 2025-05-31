package http

import (
	"encoding/json"
	"github.com/viant/agently/genai/conversation"
	agentpkg "github.com/viant/agently/genai/extension/fluxor/llm/agent"
	"log"
	"net/http"
	"strings"

	"github.com/google/uuid"
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
	mgr *conversation.Manager
}

// NewServer returns an http.Handler with routes bound.
func NewServer(mgr *conversation.Manager) http.Handler {
	s := &Server{mgr: mgr}
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/api/conversations", s.handleConversations)
	mux.HandleFunc("/v1/api/conversations/", s.handleConversationMessages) // id specific
	return mux
}

// handleConversations supports POST to create new conversation id.
func (s *Server) handleConversations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id := uuid.NewString()
	_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
}

// handleConversationMessages handles POST /v1/conversations/{id}/messages
func (s *Server) handleConversationMessages(w http.ResponseWriter, r *http.Request) {
	// expected path: /v1/conversations/{id}/messages
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/api/conversations/"), "/")
	if len(parts) < 2 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	convID := parts[0]
	if parts[1] != "messages" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodPost:
		s.handlePostMessage(w, r, convID)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

type postMessageRequest struct {
	Content string `json:"content"`
	// Optionally let client point to agent config location
	AgentLocation string `json:"agentLocation,omitempty"`
}

type postMessageResponse struct {
	Content string `json:"content"`
}

func (s *Server) handlePostMessage(w http.ResponseWriter, r *http.Request, convID string) {
	var req postMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	input := &agentpkg.QueryInput{
		ConversationID: convID,
		Query:          req.Content,
		Location:       req.AgentLocation,
	}

	ctx := tool.WithPolicy(r.Context(), &tool.Policy{Mode: tool.ModeAuto})
	out, err := s.mgr.Accept(ctx, input)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(postMessageResponse{Content: out.Content})
}

// ListenAndServe Simple helper to start the server (blocks).
func ListenAndServe(addr string, mgr *conversation.Manager) error {
	handler := NewServer(mgr)
	log.Printf("HTTP chat server listening on %s", addr)
	return http.ListenAndServe(addr, handler)
}
