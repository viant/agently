package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"errors"

	"github.com/viant/agently/adapter/http/ui"

	"github.com/viant/agently/genai/conversation"
	"github.com/viant/agently/genai/tool"

	chat "github.com/viant/agently/internal/service/chat"
	"github.com/viant/agently/metadata"

	fluxpol "github.com/viant/fluxor/policy"
	"github.com/viant/fluxor/service/approval"

	"github.com/viant/agently/internal/auth"
	d "github.com/viant/agently/internal/domain"
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
	mgr             *conversation.Manager
	titles          sync.Map // convID -> title
	toolPolicy      *tool.Policy
	fluxPolicy      *fluxpol.Policy
	chatSvc         *chat.Service
	pendingApproval approval.Service

	mu      sync.Mutex
	cancels map[string][]context.CancelFunc // convID -> cancel funcs for in-flight turns

	// Optional domain store for v1 compatibility (reads from domain instead of memory when enabled)
	store d.Store
}

// ServerOption customises HTTP server behaviour.
type ServerOption func(*Server)

// WithExecutionStore attaches an in-memory ExecutionTrace store so that GET
// /v1/api/conversations/{id}/tool-trace can return audit information.
// WithExecutionStore removed; execution traces now reconstructed from DAO tool_calls when needed.

// WithPolicies injects default tool & fluxor policies so that API requests
// inherit the configured mode (auto/ask/deny).
func WithPolicies(tp *tool.Policy, fp *fluxpol.Policy) ServerOption {
	return func(s *Server) {
		s.toolPolicy = tp
		s.fluxPolicy = fp
	}
}

// WithStore injects a domain.Store so that v1 endpoints can read from DAO-backed store
// when AGENTLY_V1_DOMAIN=1 is set. When store is nil or the flag is not set, legacy memory
// reads remain in effect.
func WithStore(st d.Store) ServerOption { return func(s *Server) { s.store = st } }

// registerCancel stores cancel so that the /terminate endpoint can abort the
// running turn for the given conversation.
func (s *Server) registerCancel(convID string, cancel context.CancelFunc) {
	if s == nil || convID == "" || cancel == nil {
		return
	}
	s.mu.Lock()
	if s.cancels == nil {
		s.cancels = make(map[string][]context.CancelFunc)
	}
	s.cancels[convID] = append(s.cancels[convID], cancel)
	s.mu.Unlock()
}

// completeCancel removes cancel from registry so memory does not leak.
func (s *Server) completeCancel(convID string, cancel context.CancelFunc) {
	if s == nil || convID == "" || cancel == nil {
		return
	}
	s.mu.Lock()
	list := s.cancels[convID]
	for i, c := range list {
		if fmt.Sprintf("%p", c) == fmt.Sprintf("%p", cancel) { // naive identity compare
			list = append(list[:i], list[i+1:]...)
			break
		}
	}
	if len(list) == 0 {
		delete(s.cancels, convID)
	} else {
		s.cancels[convID] = list
	}
	s.mu.Unlock()
}

// cancelConversation aborts all in-flight turns for convID.
func (s *Server) cancelConversation(convID string) bool {
	s.mu.Lock()
	cancels := s.cancels[convID]
	delete(s.cancels, convID)
	s.mu.Unlock()

	for _, cancel := range cancels {
		if cancel != nil {
			cancel()
		}
	}
	return len(cancels) > 0
}

// WithApprovalService injects the Fluxor approval service so that the HTTP
// callback handler can forward Accept/Decline decisions to the workflow
// engine. Supplying the service is optional — when nil the server falls back
// to chat-only status updates.
func WithApprovalService(svc approval.Service) ServerOption {
	return func(s *Server) { s.pendingApproval = svc }
}

// NewServer returns an http.Handler with routes bound.
func NewServer(mgr *conversation.Manager, opts ...ServerOption) http.Handler {
	s := &Server{mgr: mgr}
	for _, o := range opts {
		if o != nil {
			o(s)
		}
	}
	if s.store != nil {
		s.chatSvc = chat.NewService(s.store)
		s.chatSvc.AttachManager(mgr, s.toolPolicy, s.fluxPolicy)
		if s.pendingApproval != nil {
			s.chatSvc.AttachApproval(s.pendingApproval)
		}
	}
	mux := http.NewServeMux()

	// ------------------------------------------------------------------
	// Chat API (Go 1.22+ pattern based routing)
	// ------------------------------------------------------------------

	// Conversations collection
	mux.HandleFunc("POST /v1/api/conversations", s.handleConversations)     // create new conversation
	mux.HandleFunc("GET /v1/api/conversations", s.handleConversations)      // list conversations
	mux.HandleFunc("GET /v1/api/conversations/{id}", s.handleConversations) // get conversation by id

	// Conversation messages collection & item
	mux.HandleFunc("POST /v1/api/conversations/{id}/messages", func(w http.ResponseWriter, r *http.Request) {
		s.handlePostMessage(w, r, r.PathValue("id"))
	})

	mux.HandleFunc("GET /v1/api/conversations/{id}/messages", func(w http.ResponseWriter, r *http.Request) {
		s.handleGetMessages(w, r, r.PathValue("id"))
	})

	// Usage statistics
	// Usage endpoint removed; usage is computed client-side from messages.

	// Terminate/cancel current turn – best-effort cancellation of running workflow.
	mux.HandleFunc("POST /v1/api/conversations/{id}/terminate", func(w http.ResponseWriter, r *http.Request) {
		s.handleTerminateConversation(w, r, r.PathValue("id"))
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

	// ResponsePayload fetch (lazy request/response bodies)
	mux.HandleFunc("GET /v1/api/payload/{id}", func(w http.ResponseWriter, r *http.Request) {
		s.handleGetPayload(w, r, r.PathValue("id"))
	})

	// Policy approval callback (two paths for backwards-compatibility)
	mux.HandleFunc("POST /v1/api/approval/{reqId}", func(w http.ResponseWriter, r *http.Request) {
		s.handleApprovalCallback(w, r, r.PathValue("reqId"))
	})
	mux.HandleFunc("POST /approval/{reqId}", func(w http.ResponseWriter, r *http.Request) {
		s.handleApprovalCallback(w, r, r.PathValue("reqId"))
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
	ID             string `json:"id"` // message id (turn id)
	TurnID         string `json:"turnId"`
	ConversationID string `json:"conversationId"`
}

// Note: usage-related data structures moved to sdk/chat. Keep HTTP layer thin.

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

// humanTimestamp returns human friendly format like "Mon July 1st 2025, 09:15".
func humanTimestamp(t time.Time) string {
	day := t.Day()
	suffix := "th"
	if day%10 == 1 && day != 11 {
		suffix = "st"
	} else if day%10 == 2 && day != 12 {
		suffix = "nd"
	} else if day%10 == 3 && day != 13 {
		suffix = "rd"
	}
	return fmt.Sprintf("%s %s %d%s %d, %02d:%02d",
		t.Weekday().String()[:3], // Mon
		t.Month().String(),       // July
		day,
		suffix,
		t.Year(),
		t.Hour(), t.Minute())
}

// handleConversations supports POST to create new conversation id.
func (s *Server) handleConversations(w http.ResponseWriter, r *http.Request) {
	if s.chatSvc == nil {
		encode(w, http.StatusInternalServerError, nil, fmt.Errorf("chat service not initialised"))
		return
	}
	switch r.Method {
	case http.MethodPost:
		var req chat.CreateConversationRequest
		if r.Body != nil {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
				// ignore invalid body; return best-effort error below
			}
		}
		ctx := s.withAuthFromRequest(r)
		resp, err := s.chatSvc.CreateConversation(ctx, req)
		if err != nil {
			encode(w, http.StatusInternalServerError, nil, err)
			return
		}
		encode(w, http.StatusOK, resp, nil)
	case http.MethodGet:
		id := r.PathValue("id")
		if strings.TrimSpace(id) != "" {
			cv, err := s.chatSvc.GetConversation(r.Context(), id)
			if err != nil {
				encode(w, http.StatusInternalServerError, nil, err)
				return
			}
			if cv == nil {
				encode(w, http.StatusNotFound, nil, fmt.Errorf("conversation not found"))
				return
			}
			encode(w, http.StatusOK, []chat.ConversationSummary{*cv}, nil)
			return
		}
		list, err := s.chatSvc.ListConversations(s.withAuthFromRequest(r))
		if err != nil {
			encode(w, http.StatusInternalServerError, nil, err)
			return
		}
		encode(w, http.StatusOK, list, nil)
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

	// Everything else is not supported.
	w.WriteHeader(http.StatusNotFound)
}

// handleGetMessages returns conversation messages. Supported query params:
//
//	– since=<msgID>   → inclusive slice starting at msgID and including any
//	                    newer messages (generic tail-follow use-case).
//	– parentId=<id>   → (deprecated) keeps previous behaviour for one level
//	                    children filter so existing UIs do not break.
//
// When both parameters are absent the full history is returned.
// When both are provided, since has priority.
func (s *Server) handleGetMessages(w http.ResponseWriter, r *http.Request, convID string) {
	if s.chatSvc == nil {
		encode(w, http.StatusInternalServerError, nil, fmt.Errorf("chat service not initialised"))
		return
	}
	sinceId := strings.TrimSpace(r.URL.Query().Get("since"))
	includeModelCallPayload := strings.TrimSpace(r.URL.Query().Get("includeModelCallPayload"))
	resp, err := s.chatSvc.Get(r.Context(), chat.GetRequest{ConversationID: convID, SinceID: sinceId, IncludeModelCallPayload: includeModelCallPayload == "1"})
	if err != nil {
		encode(w, http.StatusInternalServerError, nil, err)
		return
	}
	encode(w, http.StatusOK, resp.Conversation, nil)
}

// handleGetPayload serves payload content or metadata for a given payload id.
// Query params:
//
//	raw=1    -> stream raw body bytes with content-type; 204 when empty
//	meta=1   -> JSON envelope without InlineBody
//	inline=1 -> JSON envelope with InlineBody when present (default)
func (s *Server) handleGetPayload(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.chatSvc == nil {
		encode(w, http.StatusInternalServerError, nil, fmt.Errorf("chat service not initialised"))
		return
	}
	raw, ctype, err := s.chatSvc.GetPayload(r.Context(), id)
	if err != nil {
		if errors.Is(err, chat.ErrNotFound) {
			encode(w, http.StatusNotFound, nil, err)
			return
		}
		if errors.Is(err, chat.ErrNoContent) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		encode(w, http.StatusInternalServerError, nil, err)
		return
	}
	w.Header().Set("Content-Type", ctype)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
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
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// handleGetUsage removed – usage route no longer exposed.

// handleElicitationCallback processes POST /v1/api/elicitation/{msgID} to
// accept or decline MCP elicitation prompts.
func (s *Server) handleElicitationCallback(w http.ResponseWriter, r *http.Request, messageID string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Action  string                 `json:"action"`
		Payload map[string]interface{} `json:"payload,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		encode(w, http.StatusBadRequest, nil, err)
		return
	}
	if s.chatSvc == nil {
		encode(w, http.StatusInternalServerError, nil, fmt.Errorf("chat service not initialised"))
		return
	}
	if err := s.chatSvc.Elicit(r.Context(), messageID, req.Action, req.Payload); err != nil {
		if strings.Contains(err.Error(), "action is required") {
			encode(w, http.StatusBadRequest, nil, err)
			return
		}
		if strings.Contains(err.Error(), "interaction message not found") {
			encode(w, http.StatusNotFound, nil, err)
			return
		}
		encode(w, http.StatusInternalServerError, nil, err)
		return
	}
	encode(w, http.StatusNoContent, nil, nil)
}

// handleApprovalCallback processes POST /v1/api/approval/{reqID} accepting or
// declining policy-approval requests that originated from the Fluxor approval
// service.
func (s *Server) handleApprovalCallback(w http.ResponseWriter, r *http.Request, messageId string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Action string `json:"action"`
		Reason string `json:"reason,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		encode(w, http.StatusBadRequest, nil, err)
		return
	}
	if strings.TrimSpace(body.Action) == "" {
		encode(w, http.StatusBadRequest, nil, fmt.Errorf("action is required"))
		return
	}
	if s.chatSvc == nil {
		encode(w, http.StatusInternalServerError, nil, fmt.Errorf("chat service not initialised"))
		return
	}
	if err := s.chatSvc.Approve(r.Context(), messageId, body.Action, body.Reason); err != nil {
		if strings.Contains(err.Error(), "interaction message not found") {
			encode(w, http.StatusNotFound, nil, err)
			return
		}
		if strings.Contains(err.Error(), "invalid action") {
			encode(w, http.StatusBadRequest, nil, err)
			return
		}
		encode(w, http.StatusInternalServerError, nil, err)
		return
	}
	encode(w, http.StatusNoContent, nil, nil)
}

// moved to sdk/chat: PostRequest and defaultLocation

func (s *Server) handlePostMessage(w http.ResponseWriter, r *http.Request, convID string) {
	if s.chatSvc == nil {
		encode(w, http.StatusInternalServerError, nil, fmt.Errorf("chat service not initialised"))
		return
	}
	var req chat.PostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		encode(w, http.StatusBadRequest, nil, err)
		return
	}
	// Preflight agent presence to return an error early instead of ACCEPTED
	if err := s.chatSvc.PreflightPost(r.Context(), convID, req); err != nil {
		encode(w, http.StatusBadRequest, nil, err)
		return
	}
	ctx := s.withAuthFromRequest(r)
	id, err := s.chatSvc.Post(ctx, convID, req)
	if err != nil {
		encode(w, http.StatusInternalServerError, nil, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(apiResponse{Status: "ACCEPTED", Data: acceptedMessage{ID: id, TurnID: id, ConversationID: convID}})
}

// handleTerminateConversation processes POST /v1/api/conversations/{id}/terminate
// and attempts to cancel all in-flight turns for the given conversation. It is
// a best-effort operation – when no turn is running the endpoint still returns
// 204 so the client can treat the conversation as idle.
func (s *Server) handleTerminateConversation(w http.ResponseWriter, r *http.Request, convID string) {
	if r.Method != http.MethodPost {
		encode(w, http.StatusMethodNotAllowed, nil, fmt.Errorf("method not allowed"))
		return
	}
	if strings.TrimSpace(convID) == "" {
		encode(w, http.StatusBadRequest, nil, fmt.Errorf("conversation id required"))
		return
	}

	cancelled := false
	if s.chatSvc != nil {
		cancelled = s.chatSvc.Cancel(convID)
	}

	status := http.StatusAccepted
	if !cancelled {
		status = http.StatusNoContent
	}

	encode(w, status, map[string]any{"cancelled": cancelled}, nil)
}

// ListenAndServe Simple helper to start the server (blocks).
func ListenAndServe(addr string, mgr *conversation.Manager) error {
	handler := NewServer(mgr)
	log.Printf("HTTP chat server listening on %s", addr)
	return http.ListenAndServe(addr, handler)
}

// withAuthFromRequest extracts Authorization bearer token, decodes minimal
// identity and enriches the request context so downstream services can persist
// user info without direct HTTP coupling.
func (s *Server) withAuthFromRequest(r *http.Request) context.Context {
	ctx := context.Background()
	authz := r.Header.Get("Authorization")
	if strings.TrimSpace(authz) == "" {
		// No auth header; set default anonymous user for flow testing
		return auth.WithUserInfo(ctx, &auth.UserInfo{Subject: "anonymous"})
	}
	token := auth.ExtractBearer(authz)
	if token == "" {
		return auth.WithUserInfo(ctx, &auth.UserInfo{Subject: "anonymous"})
	}
	ctx = auth.WithBearer(ctx, token)
	if info, err := auth.DecodeUserInfo(token); err == nil && info != nil {
		ctx = auth.WithUserInfo(ctx, info)
		return ctx
	}
	// Could not decode – use anonymous by default
	return auth.WithUserInfo(ctx, &auth.UserInfo{Subject: "anonymous"})
}
