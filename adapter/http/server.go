package http

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/viant/agently/adapter/http/ui"
	mcpclient "github.com/viant/agently/adapter/mcp"
	"github.com/viant/agently/genai/conversation"
	agentpkg "github.com/viant/agently/genai/extension/fluxor/llm/agent"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/tool"
	"github.com/viant/agently/metadata"
	"github.com/viant/mcp-protocol/schema"

	"github.com/google/uuid"
)

// Server wraps a conversation manager and exposes minimal REST endpoints:
//
//	POST /v1/conversations                 -> {"id": "..."}
//	POST /v1/conversations/{id}/messages         -> accept user message, returns {"id": "..."} (202 Accepted)
//	GET  /v1/conversations/{id}/messages         -> full history
//	GET  /v1/conversations/{id}/messages/{msgID} -> single message by ID
//
// The server is designed to be simple and lightweight, suitable for quick
type Server struct {
	mgr            *conversation.Manager
	executionStore *memory.ExecutionStore
	titles         sync.Map // convID -> title
}

// ServerOption customises HTTP server behaviour.
type ServerOption func(*Server)

// WithExecutionStore attaches an in-memory ExecutionTrace store so that GET
// /v1/api/conversations/{id}/tool-trace can return audit information.
func WithExecutionStore(ts *memory.ExecutionStore) ServerOption {
	return func(s *Server) { s.executionStore = ts }
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

	// ------------------------------------------------------------------
	// Chat API (Go 1.22+ pattern based routing)
	// ------------------------------------------------------------------

	// Conversations collection
	mux.HandleFunc("POST /v1/api/conversations", s.handleConversations)     // create new conversation
	mux.HandleFunc("GET /v1/api/conversations", s.handleConversations)      // list conversations
	mux.HandleFunc("GET /v1/api/conversations/{id}", s.handleConversations) // list conversations

	// Conversation messages collection & item
	mux.HandleFunc("POST /v1/api/conversations/{id}/messages", func(w http.ResponseWriter, r *http.Request) {
		s.handlePostMessage(w, r, r.PathValue("id"))
	})

	mux.HandleFunc("GET /v1/api/conversations/{id}/messages", func(w http.ResponseWriter, r *http.Request) {
		s.handleGetMessages(w, r, r.PathValue("id"))
	})

	mux.HandleFunc("GET /v1/api/conversations/{id}/messages/{msgId}", func(w http.ResponseWriter, r *http.Request) {
		s.handleGetSingleMessage(w, r, r.PathValue("id"), r.PathValue("msgId"))
	})

	// Executions trace
	mux.HandleFunc("GET /v1/api/conversations/{id}/execution", func(w http.ResponseWriter, r *http.Request) {
		s.handleGetExecution(w, r, r.PathValue("id"))
	})

	// ------------------------------------------------------------------
	// Forge UI metadata endpoints
	// ------------------------------------------------------------------
	// Serve UI metadata from embedded YAML definitions.
	uiRoot := "embed://localhost/"
	uiHandler := ui.NewEmbeddedHandler(uiRoot, &metadata.FS)
	mux.Handle("/v1/api/agently/forge/", http.StripPrefix("/v1/api/agently/forge", uiHandler))

	// Elicitation callback (MCP interactive)
	mux.HandleFunc("POST /v1/api/elicitation/{msgId}", func(w http.ResponseWriter, r *http.Request) {
		s.handleElicitationCallback(w, r, r.PathValue("msgId"))
	})

	// User interaction callback
	mux.HandleFunc("POST /v1/api/interaction/{msgId}", func(w http.ResponseWriter, r *http.Request) {
		s.handleInteractionCallback(w, r, r.PathValue("msgId"))
	})

	return WithCORS(mux)
}

// apiResponse is the unified wrapper returned by all Agently HTTP endpoints.
type apiResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}

// ------------------------------
// Typed payload structs (avoid map)
// ------------------------------

type conversationInfo struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type acceptedMessage struct {
	ID string `json:"id"`
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
	status := "ok"
	if statusCode == http.StatusProcessing {
		status = "processing"
	}

	_ = json.NewEncoder(w).Encode(apiResponse{Status: status, Data: data})
}

// handleConversations supports POST to create new conversation id.
func (s *Server) handleConversations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		id := uuid.NewString()
		title := fmt.Sprintf("Conversation %s", time.Now().Format("2006-01-02 15:04:05"))
		s.titles.Store(id, title)
		encode(w, http.StatusOK, conversationInfo{ID: id, Title: title}, nil)
	case http.MethodGet:
		id := r.PathValue("id")
		var err error
		var ids []string
		if id != "" {
			ids = append(ids, id)
		}
		if len(ids) == 0 {
			ids, err = s.mgr.List(r.Context())
			if err != nil {
				encode(w, http.StatusInternalServerError, nil, err)
				return
			}
		}
		conversations := make([]conversationInfo, len(ids))
		for i, id := range ids {
			title, _ := s.titles.Load(id)
			var t string
			if titleStr, ok := title.(string); ok {
				t = titleStr
			}
			if strings.TrimSpace(t) == "" {
				t = id
			}
			conversations[i] = conversationInfo{ID: id, Title: t}
		}
		encode(w, http.StatusOK, conversations, nil)
	default:
		encode(w, http.StatusMethodNotAllowed, nil, fmt.Errorf("method not allowed"))
	}
}

// handleConversationMessages handles POST /v1/conversations/{id}/messages
func (s *Server) handleConversationMessages(w http.ResponseWriter, r *http.Request, convID string, extraParts []string) {
	// When extraParts is empty we are dealing with collection operations
	if len(extraParts) == 0 {
		switch r.Method {
		case http.MethodPost:
			s.handlePostMessage(w, r, convID)
		case http.MethodGet:
			s.handleGetMessages(w, r, convID)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
		return
	}

	// If we have exactly one additional part treat it as message ID for retrieval.
	if len(extraParts) == 1 && r.Method == http.MethodGet {
		msgID := extraParts[0]
		s.handleGetSingleMessage(w, r, convID, msgID)
		return
	}

	// Everything else is not supported.
	w.WriteHeader(http.StatusNotFound)
}

// handleGetMessages supports GET /v1/api/conversations/{id}/messages to return full history.
func (s *Server) handleGetMessages(w http.ResponseWriter, r *http.Request, convID string) {
	parentId := r.URL.Query().Get("parentId")
	msgs, err := s.mgr.Messages(r.Context(), convID, parentId)
	if err != nil {
		encode(w, http.StatusInternalServerError, nil, err)
		return
	}
	if len(msgs) == 0 {
		encode(w, http.StatusProcessing, msgs, nil)
		return
	}

	// Filter out resolved MCP elicitation messages (status != "open")
	var filtered []memory.Message
	for _, m := range msgs {
		if m.Role == "mcpelicitation" && m.Status != "" && m.Status != "open" {
			continue
		}
		filtered = append(filtered, m)
	}

	encode(w, http.StatusOK, filtered, nil)
}

// handleGetSingleMessage supports GET /v1/api/conversations/{id}/messages/{msgID}
func (s *Server) handleGetSingleMessage(w http.ResponseWriter, r *http.Request, convID, msgID string) {
	if msgID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	msgs, err := s.mgr.Messages(r.Context(), convID, "")
	if err != nil {
		encode(w, http.StatusInternalServerError, nil, err)
		return
	}

	for _, m := range msgs {
		if m.ID == msgID {
			encode(w, http.StatusOK, m, nil)
			return
		}
	}

	// Not found.
	w.WriteHeader(http.StatusNotFound)
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
		// Pass remaining parts (after "messages") to specialised handler
		s.handleConversationMessages(w, r, convID, parts[2:])
	case "execution":
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		s.handleGetExecution(w, r, convID)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// handleGetExecution responds with the full list of ExecutionTrace entries for the
// given conversation ID.
func (s *Server) handleGetExecution(w http.ResponseWriter, r *http.Request, convID string) {
	if s.executionStore == nil {
		// Trace store not hooked up – return empty slice for compatibility.
		encode(w, http.StatusOK, []memory.ExecutionTrace{}, nil)
		return
	}
	// If the client supplied a parentId query parameter, filter the trace list
	// so that callers can retrieve only the tool invocations associated with
	// a specific assistant message.
	query := r.URL.Query()
	parentID := query.Get("parentId")
	if format := query.Get("format"); format == "outcome" {
		if outcomes, err := s.executionStore.ListOutcome(r.Context(), convID, parentID); err == nil {
			encode(w, http.StatusOK, outcomes, nil)
			return
		}
	}
	if parentID != "" {
		traces, err := s.executionStore.ListByParent(r.Context(), convID, parentID)
		if err != nil {
			encode(w, http.StatusInternalServerError, nil, err)
			return
		}
		encode(w, http.StatusOK, traces, nil)
		return
		// When conversion fails, fall back to returning full list – the caller
		// can still filter client-side.
	}

	traces, err := s.executionStore.List(r.Context(), convID)
	if err != nil {
		encode(w, http.StatusInternalServerError, nil, err)
		return
	}
	encode(w, http.StatusOK, traces, nil)
}

// handleElicitationCallback processes POST /v1/api/elicitation/{msgID} to
// accept or decline MCP elicitation prompts.
func (s *Server) handleElicitationCallback(w http.ResponseWriter, r *http.Request, msgID string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Action  string                 `json:"action"` // "accept" | "decline" | "cancel"
		Payload map[string]interface{} `json:"payload,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		encode(w, http.StatusBadRequest, nil, err)
		return
	}

	if req.Action == "" {
		encode(w, http.StatusBadRequest, nil, fmt.Errorf("action is required"))
		return
	}

	// Find the conversation containing this message.
	ids, err := s.mgr.List(r.Context())
	if err != nil {
		encode(w, http.StatusInternalServerError, nil, err)
		return
	}

	var convID string
	var found bool
	for _, id := range ids {
		msgs, _ := s.mgr.Messages(r.Context(), id, "")
		for _, m := range msgs {
			if m.ID == msgID {
				convID = id
				found = true
				break
			}
		}
		if found {
			break
		}
	}

	if !found {
		encode(w, http.StatusNotFound, nil, fmt.Errorf("elicitation message not found"))
		return
	}

	// Update message status.
	status := "declined"
	if req.Action == "accept" {
		status = "done"
	}

	_ = s.mgr.History().UpdateMessage(r.Context(), convID, msgID, func(m *memory.Message) {
		m.Status = status
	})

	if ch, ok := mcpclient.Waiter(msgID); ok {
		result := &schema.ElicitResult{ // import schema
			Action:  schema.ElicitResultAction(req.Action),
			Content: req.Payload,
		}
		ch <- result
	}

	// TODO: bridge to MCP awaiter if present (not implemented here)

	encode(w, http.StatusNoContent, nil, nil)
}

// handleInteractionCallback processes POST /v1/api/interaction/{msgID} to
// accept or decline MCP user-interaction prompts.
func (s *Server) handleInteractionCallback(w http.ResponseWriter, r *http.Request, msgID string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Action string `json:"action"` // accept | decline | cancel
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		encode(w, http.StatusBadRequest, nil, err)
		return
	}
	if req.Action == "" {
		encode(w, http.StatusBadRequest, nil, fmt.Errorf("action is required"))
		return
	}

	// Find conversation containing this message
	ids, err := s.mgr.List(r.Context())
	if err != nil {
		encode(w, http.StatusInternalServerError, nil, err)
		return
	}

	var convID string
	var found bool
	for _, id := range ids {
		msgs, _ := s.mgr.Messages(r.Context(), id, "")
		for _, m := range msgs {
			if m.ID == msgID {
				convID = id
				found = true
				break
			}
		}
		if found {
			break
		}
	}

	if !found {
		encode(w, http.StatusNotFound, nil, fmt.Errorf("interaction message not found"))
		return
	}

	status := "declined"
	if req.Action == "accept" {
		status = "done"
	}

	_ = s.mgr.History().UpdateMessage(r.Context(), convID, msgID, func(m *memory.Message) {
		m.Status = status
	})

	if ch, ok := mcpclient.WaiterInteraction(msgID); ok {
		ch <- &schema.CreateUserInteractionResult{}
	}

	encode(w, http.StatusNoContent, nil, nil)
}

type postMessageRequest struct {
	Content string   `json:"content"`
	Agent   string   `json:"agent,omitempty"`
	Model   string   `json:"model,omitempty"`
	Tools   []string `json:"tools,omitempty"`
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
		Location:       defaultLocation(req.Agent),
		ModelOverride:  req.Model,
		ToolsAllowed:   req.Tools,
		MessageID:      uuid.New().String(),
	}

	// Kick off processing in the background so that we can respond immediately.
	originalCtx := r.Context()
	policy := tool.FromContext(originalCtx)
	go func() {
		// Detach from request context to avoid immediate cancellation.
		ctx := context.Background()
		// Carry conversation ID so downstream services (MCP client) can
		// persist interactive prompts even before the workflow explicitly
		// sets it again.
		ctx = context.WithValue(ctx, memory.ConversationIDKey, convID)

		// Always auto mode for API requests; no CLI prompts.
		ctx = tool.WithPolicy(ctx, &tool.Policy{Mode: tool.ModeAuto})
		if policy != nil {
			ctx = tool.WithPolicy(ctx, policy)
		}
		if _, err := s.mgr.Accept(ctx, input); err != nil {
			log.Printf("async accept error: %v", err)
		}
	}()

	// Inform the caller that the message has been accepted and is being processed.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(apiResponse{Status: "ACCEPTED", Data: acceptedMessage{ID: input.MessageID}})
}

// ListenAndServe Simple helper to start the server (blocks).
func ListenAndServe(addr string, mgr *conversation.Manager) error {
	handler := NewServer(mgr)
	log.Printf("HTTP chat server listening on %s", addr)
	return http.ListenAndServe(addr, handler)
}
