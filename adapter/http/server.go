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
	fluxpol "github.com/viant/fluxor/policy"
	"github.com/viant/fluxor/service/approval"
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

	toolPolicy  *tool.Policy
	fluxPolicy  *fluxpol.Policy
	approvalSvc approval.Service
}

// ServerOption customises HTTP server behaviour.
type ServerOption func(*Server)

// WithExecutionStore attaches an in-memory ExecutionTrace store so that GET
// /v1/api/conversations/{id}/tool-trace can return audit information.
func WithExecutionStore(ts *memory.ExecutionStore) ServerOption {
	return func(s *Server) { s.executionStore = ts }
}

// WithPolicies injects default tool & fluxor policies so that API requests
// inherit the configured mode (auto/ask/deny).
func WithPolicies(tp *tool.Policy, fp *fluxpol.Policy) ServerOption {
	return func(s *Server) {
		s.toolPolicy = tp
		s.fluxPolicy = fp
	}
}

// WithApprovalService injects the Fluxor approval service so that the HTTP
// callback handler can forward Accept/Decline decisions to the workflow
// engine. Supplying the service is optional — when nil the server falls back
// to chat-only status updates.
func WithApprovalService(svc approval.Service) ServerOption {
	return func(s *Server) { s.approvalSvc = svc }
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
	mux.HandleFunc("POST /v1/api/conversations/", s.handleConversations)    // create (trailing slash)
	mux.HandleFunc("GET /v1/api/conversations", s.handleConversations)      // list conversations
	mux.HandleFunc("GET /v1/api/conversations/", s.handleConversations)     // list (trailing slash)
	mux.HandleFunc("GET /v1/api/conversations/{id}", s.handleConversations) // get conversation by id

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

	// Usage statistics
	mux.HandleFunc("GET /v1/api/conversations/{id}/usage", func(w http.ResponseWriter, r *http.Request) {
		s.handleGetUsage(w, r, r.PathValue("id"))
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

		// Ensure the new conversation appears in subsequent list responses by
		// inserting an empty slice into the history store. We do this by adding a
		// zero-length message array entry so that ListIDs picks up the new key
		// without polluting the visible chat history.
		if hs, ok := s.mgr.History().(*memory.HistoryStore); ok {
			hs.EnsureConversation(id)
		}

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
	data, _ := json.Marshal(msgs)
	fmt.Println("MESSAGES", string(data))

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
		switch m.Role {
		case "mcpelicitation", "mcpuserinteraction", "policyapproval":
			if m.Status != "" && m.Status != "open" {
				continue
			}
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
	msg, err := s.mgr.History().LookupMessage(r.Context(), msgID)
	if err != nil {
		encode(w, http.StatusInternalServerError, nil, err)
		return
	}
	if msg == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	encode(w, http.StatusOK, msg, nil)
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

// handleGetUsage responds with aggregated + per-model token usage.
func (s *Server) handleGetUsage(w http.ResponseWriter, r *http.Request, convID string) {
	// Require GET only.
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	uStore := s.mgr.UsageStore()
	if uStore == nil {
		// No usage tracking configured – return zeros so that callers don't error out.
		encode(w, http.StatusOK, map[string]any{
			"inputTokens":     0,
			"outputTokens":    0,
			"embeddingTokens": 0,
			"perModel":        map[string]any{},
		}, nil)
		return
	}

	p, c, e := uStore.Totals(convID)
	per := map[string]any{}
	if agg := uStore.Aggregator(convID); agg != nil {
		for _, model := range agg.Keys() {
			st := agg.PerModel[model]
			if st == nil {
				continue
			}
			per[model] = map[string]int{
				"inputTokens":     st.PromptTokens,
				"outputTokens":    st.CompletionTokens,
				"embeddingTokens": st.EmbeddingTokens,
			}
		}
	}

	encode(w, http.StatusOK, map[string]any{
		"inputTokens":     p,
		"outputTokens":    c,
		"embeddingTokens": e,
		"perModel":        per,
	}, nil)
}

// handleElicitationCallback processes POST /v1/api/elicitation/{msgID} to
// accept or decline MCP elicitation prompts.
func (s *Server) handleElicitationCallback(w http.ResponseWriter, r *http.Request, messageID string) {
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

	message, _ := s.mgr.History().LookupMessage(r.Context(), messageID)
	if message == nil {
		encode(w, http.StatusNotFound, nil, fmt.Errorf("interaction message not found"))
		return
	}

	// Find the conversation containing this message.

	// Update message status.
	status := "declined"
	if req.Action == "accept" {
		status = "done"
	}

	_ = s.mgr.History().UpdateMessage(r.Context(), messageID, func(m *memory.Message) {
		m.Status = status
	})

	if ch, ok := mcpclient.Waiter(messageID); ok {
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
func (s *Server) handleInteractionCallback(w http.ResponseWriter, r *http.Request, messageID string) {
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

	message, _ := s.mgr.History().LookupMessage(r.Context(), messageID)
	if message == nil {
		encode(w, http.StatusNotFound, nil, fmt.Errorf("interaction message not found"))
		return
	}

	status := "declined"
	if req.Action == "accept" {
		status = "done"
	}

	_ = s.mgr.History().UpdateMessage(r.Context(), messageID, func(m *memory.Message) {
		m.Status = status
	})

	if ch, ok := mcpclient.WaiterInteraction(messageID); ok {
		ch <- &schema.CreateUserInteractionResult{}
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
		Action string `json:"action"` // accept | decline | cancel
		Reason string `json:"reason,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		encode(w, http.StatusBadRequest, nil, err)
		return
	}
	if body.Action == "" {
		encode(w, http.StatusBadRequest, nil, fmt.Errorf("action is required"))
		return
	}

	// Map action to approved boolean accepting both "accept" (legacy) and
	// "approve" (current Forge UI wording) for forward-compatibility.
	var approved bool
	switch strings.ToLower(strings.TrimSpace(body.Action)) {
	case "accept", "approve", "approved", "yes", "y":
		approved = true
	case "decline", "deny", "reject", "no", "n":
		approved = false
	case "cancel":
		// Treat cancel same as decline at approval layer but keep message open.
		encode(w, http.StatusNoContent, nil, nil)
		return
	default:
		encode(w, http.StatusBadRequest, nil, fmt.Errorf("invalid action"))
		return
	}

	message, _ := s.mgr.History().LookupMessage(r.Context(), messageId)
	if message == nil {
		encode(w, http.StatusNotFound, nil, fmt.Errorf("interaction message not found"))
		return
	}

	// 1) Persist decision in Conversation history so UI updates immediately.
	newStatus := "declined"
	if approved {
		newStatus = "done"
	}

	fmt.Printf("MESSAGE ID: %s\n", messageId)
	_ = s.mgr.History().UpdateMessage(r.Context(), messageId, func(m *memory.Message) {
		m.Status = newStatus
	})

	// 2) Forward the decision to the Fluxor approval service so that the
	//    workflow waiting on this request can resume.  When the service is
	//    not provided (nil) we simply skip this step – the workflow would be
	//    blocked, but the UI still reflects the user choice.
	if s.approvalSvc != nil {
		_, _ = s.approvalSvc.Decide(r.Context(), messageId, approved, body.Reason)
	}

	encode(w, http.StatusNoContent, nil, nil)
}

type postMessageRequest struct {
	Content string                 `json:"content"`
	Agent   string                 `json:"agent,omitempty"`
	Model   string                 `json:"model,omitempty"`
	Tools   []string               `json:"tools,omitempty"`
	Context map[string]interface{} `json:"context,omitempty"`
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
		AgentName:      defaultLocation(req.Agent),
		ModelOverride:  req.Model,
		ToolsAllowed:   req.Tools,
		Context:        req.Context,
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

		// Apply server-level default tool policy (auto, ask, deny). Fallback
		// to auto when not provided.
		if s.toolPolicy != nil {
			ctx = tool.WithPolicy(ctx, s.toolPolicy)
		} else {
			ctx = tool.WithPolicy(ctx, &tool.Policy{Mode: tool.ModeAuto})
		}
		if policy != nil {
			ctx = tool.WithPolicy(ctx, policy)
		}

		// Also embed fluxor policy so workflow approval layer matches the
		// configured mode.
		if s.fluxPolicy != nil {
			ctx = fluxpol.WithPolicy(ctx, s.fluxPolicy)
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
