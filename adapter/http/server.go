package http

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/viant/agently/adapter/http/ui"
	mcpclient "github.com/viant/agently/adapter/mcp"
	"github.com/viant/agently/genai/conversation"
	agentpkg "github.com/viant/agently/genai/extension/fluxor/llm/agent"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/stage"
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

	mu      sync.Mutex
	cancels map[string][]context.CancelFunc // convID -> cancel funcs for in-flight turns
}

// currentStage returns pointer to Stage snapshot for convID when available.
func (s *Server) currentStage(convID string) *stage.Stage {
	if s == nil || convID == "" {
		return nil
	}
	if s.mgr == nil {
		return nil
	}
	if ss := s.mgr.StageStore(); ss != nil {
		return ss.Get(convID)
	}
	return nil
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

	// Execution part (request/response) – heavy payload on-demand
	mux.HandleFunc("GET /v1/api/conversations/{id}/execution/{traceId}/{part}", func(w http.ResponseWriter, r *http.Request) {
		traceIDStr := r.PathValue("traceId")
		part := r.PathValue("part") // "request" | "response"
		s.handleGetExecutionPart(w, r, r.PathValue("id"), traceIDStr, part)
	})

	// Usage statistics
	mux.HandleFunc("GET /v1/api/conversations/{id}/usage", func(w http.ResponseWriter, r *http.Request) {
		s.handleGetUsage(w, r, r.PathValue("id"))
	})

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

	// Inline elicitation refinement (preset override)
	mux.HandleFunc("POST /v1/api/elicitation/{msgId}/refine", func(w http.ResponseWriter, r *http.Request) {
		s.handleElicitationRefine(w, r, r.PathValue("msgId"))
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
	Status  string       `json:"status"`
	Message string       `json:"message,omitempty"`
	Stage   *stage.Stage `json:"stage,omitempty"`
	Data    any          `json:"data,omitempty"`
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

// usagePerModel represents token statistics per single model.
type usagePerModel struct {
	Model           string `json:"model"`
	InputTokens     int    `json:"inputTokens"`
	OutputTokens    int    `json:"outputTokens"`
	EmbeddingTokens int    `json:"embeddingTokens"`
	CachedTokens    int    `json:"cachedTokens"`
}

// usagePayload is the aggregate returned by GET /v1/api/conversations/{id}/usage
// wrapped in the standard API envelope.
type usagePayload struct {
	ConversationID  string          `json:"conversationId"`
	InputTokens     int             `json:"inputTokens"`
	OutputTokens    int             `json:"outputTokens"`
	EmbeddingTokens int             `json:"embeddingTokens"`
	CachedTokens    int             `json:"cachedTokens"`
	TotalTokens     int             `json:"totalTokens"`
	PerModel        []usagePerModel `json:"perModel"`
}

// UsageResponse represents the full JSON body returned by GET
// /v1/api/conversations/{id}/usage after the standard API envelope
// encoding. It mirrors the {status, data:[usagePayload]} structure so that
// callers can marshal/unmarshal strongly typed objects instead of working
// with generic maps.
type UsageResponse struct {
	Status string         `json:"status"`
	Data   []usagePayload `json:"data"`
}

// encode writes JSON response with the unified structure.
func encode(w http.ResponseWriter, statusCode int, data interface{}, err error, st *stage.Stage) {
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

	_ = json.NewEncoder(w).Encode(apiResponse{Status: status, Stage: st, Data: data})
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
	switch r.Method {
	case http.MethodPost:
		id := uuid.NewString()
		title := fmt.Sprintf("Conversation at %s", humanTimestamp(time.Now()))

		// Read optional overrides from request body
		payload := map[string]any{}
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&payload) // tolerate empty / invalid body
		}
		payload["id"] = id
		payload["title"] = title
		payload["createdAt"] = time.Now().UTC().Format(time.RFC3339)

		s.titles.Store(id, title)

		// ensure key exists in history store for subsequent list queries
		if hs, ok := s.mgr.History().(*memory.HistoryStore); ok {
			hs.EnsureConversation(id)
		}

		encode(w, http.StatusOK, payload, nil, nil)
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
				encode(w, http.StatusInternalServerError, nil, err, nil)
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
		encode(w, http.StatusOK, conversations, nil, nil)
	default:
		encode(w, http.StatusMethodNotAllowed, nil, fmt.Errorf("method not allowed"), nil)
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
	// ------------------------------------------------------------------
	// 1. Determine filter semantics
	// ------------------------------------------------------------------
	sinceId := strings.TrimSpace(r.URL.Query().Get("since"))
	parentId := strings.TrimSpace(r.URL.Query().Get("parentId")) // legacy

	// ------------------------------------------------------------------
	// 2. Fetch from manager (enriched with execution traces)
	// ------------------------------------------------------------------
	var msgs []memory.Message
	var err error

	if sinceId == "" {
		// legacy or full history path
		msgs, err = s.mgr.Messages(r.Context(), convID, parentId)
	} else {
		// Retrieve full history, then slice – keeps enrichment logic intact.
		var all []memory.Message
		all, err = s.mgr.Messages(r.Context(), convID, "")
		if err == nil {
			// Find index of sinceId (inclusive)
			start := -1
			for i, m := range all {
				if m.ID == sinceId {
					start = i
					break
				}
			}
			if start == -1 {
				// Message not yet available – return 102 Processing but include current stage.
				encode(w, http.StatusProcessing, []memory.Message{}, nil, s.currentStage(convID))
				return
			}
			msgs = all[start:]
		}
	}
	if err != nil {
		encode(w, http.StatusInternalServerError, nil, err, nil)
		return
	}
	if len(msgs) == 0 {
		// No new messages but still return current stage so UI can update.
		encode(w, http.StatusProcessing, msgs, nil, s.currentStage(convID))
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

	st := s.currentStage(convID)
	if st != nil && st.Phase == stage.StageThinking {
		// Heuristic: presence of tool or assistant message indicates progress.
		last := filtered[len(filtered)-1]
		switch last.Role {
		case "tool":
			if last.ToolName != nil {
				name := strings.ToLower(*last.ToolName)
				if name == "llm/core.generate" || name == "llm/core:generate" {
					// Treat generate-as-tool as part of thinking, not executing.
					st = &stage.Stage{Phase: stage.StageThinking}
				} else {
					st = &stage.Stage{Phase: stage.StageExecuting, Tool: *last.ToolName}
				}
			} else {
				st = &stage.Stage{Phase: stage.StageExecuting}
			}
		case "assistant":
			st = &stage.Stage{Phase: stage.StageDone}
		}
	}
	encode(w, http.StatusOK, filtered, nil, st)

}

// handleGetSingleMessage supports GET /v1/api/conversations/{id}/messages/{msgID}
func (s *Server) handleGetSingleMessage(w http.ResponseWriter, r *http.Request, convID, msgID string) {
	if msgID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	msg, err := s.mgr.History().LookupMessage(r.Context(), msgID)
	if err != nil {
		encode(w, http.StatusInternalServerError, nil, err, nil)
		return
	}
	if msg == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	encode(w, http.StatusOK, msg, nil, nil)
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
		encode(w, http.StatusOK, []memory.ExecutionTrace{}, nil, nil)
		return
	}
	// If the client supplied a parentId query parameter, filter the trace list
	// so that callers can retrieve only the tool invocations associated with
	// a specific assistant message.
	query := r.URL.Query()
	parentID := query.Get("parentId")
	if format := query.Get("format"); format == "outcome" {
		if outcomes, err := s.executionStore.ListOutcome(r.Context(), convID, parentID); err == nil {
			encode(w, http.StatusOK, outcomes, nil, nil)
			return
		}
	}
	if parentID != "" {
		traces, err := s.executionStore.ListByParent(r.Context(), convID, parentID)
		if err != nil {
			encode(w, http.StatusInternalServerError, nil, err, nil)
			return
		}
		encode(w, http.StatusOK, traces, nil, nil)
		return
		// When conversion fails, fall back to returning full list – the caller
		// can still filter client-side.
	}

	traces, err := s.executionStore.List(r.Context(), convID)
	if err != nil {
		encode(w, http.StatusInternalServerError, nil, err, nil)
		return
	}

	// Strip heavy request/response payloads to keep response lightweight.
	lite := make([]*memory.ExecutionTrace, len(traces))
	for i, tr := range traces {
		if tr == nil {
			continue
		}
		clone := *tr
		clone.Request = nil
		clone.Result = nil
		lite[i] = &clone
	}
	encode(w, http.StatusOK, lite, nil, nil)
}

// handleGetExecutionPart serves either the stored Request or Response payload
// for a specific execution-trace entry. Large blobs are therefore transferred
// only when the user explicitly expands the details in the UI.
func (s *Server) handleGetExecutionPart(w http.ResponseWriter, r *http.Request, convID string, traceIDStr string, part string) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if s.executionStore == nil {
		encode(w, http.StatusNotFound, nil, fmt.Errorf("execution store not configured"), nil)
		return
	}

	id64, err := strconv.ParseInt(traceIDStr, 10, 0)
	if err != nil || id64 <= 0 {
		encode(w, http.StatusBadRequest, nil, fmt.Errorf("invalid trace id"), nil)
		return
	}

	trace, _ := s.executionStore.Get(r.Context(), convID, int(id64))
	if trace == nil {
		encode(w, http.StatusNotFound, nil, fmt.Errorf("trace not found"), nil)
		return
	}

	switch strings.ToLower(part) {
	case "request":
		if trace.Request == nil {
			encode(w, http.StatusNoContent, nil, nil, nil)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(trace.Request)
	case "response":
		if trace.Result == nil {
			encode(w, http.StatusNoContent, nil, nil, nil)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(trace.Result)
	default:
		encode(w, http.StatusNotFound, nil, fmt.Errorf("unknown part"), nil)
	}
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
		encode(w, http.StatusOK, []usagePayload{{
			ConversationID:  convID,
			InputTokens:     0,
			OutputTokens:    0,
			EmbeddingTokens: 0,
			CachedTokens:    0,
			TotalTokens:     0,
			PerModel:        []usagePerModel{},
		}}, nil, nil)
		return
	}

	// ------------------------------------------------------------------
	// Build aggregated totals and per-model breakdown in the new response
	// structure expected by Forge UI. The JSON payload now looks like:
	// {"data":[ { totals…, "perModel":[ {model: "…", …} ] } ]}
	// ------------------------------------------------------------------

	// Totals across all models.
	p, c, e, cached := uStore.Totals(convID)

	// Build deterministic slice of per-model statistics.
	perModels := make([]usagePerModel, 0)
	if agg := uStore.Aggregator(convID); agg != nil {
		for _, model := range agg.Keys() {
			st := agg.PerModel[model]
			if st == nil {
				continue
			}
			perModels = append(perModels, usagePerModel{
				Model:           model,
				InputTokens:     st.PromptTokens,
				OutputTokens:    st.CompletionTokens,
				EmbeddingTokens: st.EmbeddingTokens,
				CachedTokens:    st.CachedTokens,
			})
		}
	}

	aggPayload := usagePayload{
		ConversationID:  convID,
		InputTokens:     p,
		OutputTokens:    c,
		EmbeddingTokens: e,
		CachedTokens:    cached,
		TotalTokens:     p + c + e + cached,
		PerModel:        perModels,
	}

	encode(w, http.StatusOK, []usagePayload{aggPayload}, nil, nil)
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
		encode(w, http.StatusBadRequest, nil, err, nil)
		return
	}

	if req.Action == "" {
		encode(w, http.StatusBadRequest, nil, fmt.Errorf("action is required"), nil)
		return
	}

	message, _ := s.mgr.History().LookupMessage(r.Context(), messageID)
	if message == nil {
		encode(w, http.StatusNotFound, nil, fmt.Errorf("interaction message not found"), nil)
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

	encode(w, http.StatusNoContent, nil, nil, nil)
}

// handleElicitationRefine processes POST /v1/api/elicitation/{msgID}/refine.
// It allows the client (UI) to supply a preset refinement that customises
// field ordering or widget overrides for the elicitation form. The endpoint
// mutates the stored assistant message in-place so that subsequent GET
// /messages polls return the refined schema.
func (s *Server) handleElicitationRefine(w http.ResponseWriter, r *http.Request, messageID string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Parse payload – we accept the same structure as plan.Elicitation for
	// simplicity and trust the UI to supply valid data.
	var refine struct {
		Fields []map[string]any `json:"fields"`
		UI     map[string]any   `json:"ui,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&refine); err != nil {
		encode(w, http.StatusBadRequest, nil, err, nil)
		return
	}

	if len(refine.Fields) == 0 {
		encode(w, http.StatusBadRequest, nil, fmt.Errorf("fields are required"), nil)
		return
	}

	// TODO: apply refinement to elicitation schema (Phase 2b). For now we just
	// acknowledge so UI knows the preset was accepted.
	encode(w, http.StatusNoContent, nil, nil, nil)
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
		encode(w, http.StatusBadRequest, nil, err, nil)
		return
	}
	if body.Action == "" {
		encode(w, http.StatusBadRequest, nil, fmt.Errorf("action is required"), nil)
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
		encode(w, http.StatusNoContent, nil, nil, nil)
		return
	default:
		encode(w, http.StatusBadRequest, nil, fmt.Errorf("invalid action"), nil)
		return
	}

	message, _ := s.mgr.History().LookupMessage(r.Context(), messageId)
	if message == nil {
		encode(w, http.StatusNotFound, nil, fmt.Errorf("interaction message not found"), nil)
		return
	}

	// 1) Persist decision in Conversation history so UI updates immediately.
	newStatus := "declined"
	if approved {
		newStatus = "done"
	}

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

	encode(w, http.StatusNoContent, nil, nil, nil)
}

type postMessageRequest struct {
	Content string                 `json:"content"`
	Agent   string                 `json:"agent,omitempty"`
	Model   string                 `json:"model,omitempty"`
	Tools   []string               `json:"tools,omitempty"`
	Context map[string]interface{} `json:"context,omitempty"`
}

// defaultLocation returns supplied if not empty otherwise "chat".
// defaultLocation preserves explicit agent location when provided. Returning
// empty string lets the downstream agent-service fall back to the per-
// conversation Agent stored in memory (ConversationMeta).  We therefore no
// longer substitute the hard-coded "chat" default here; the agent service
// already has its own default when no agent can be resolved.
func defaultLocation(loc string) string {
	return strings.TrimSpace(loc)
}

func (s *Server) handlePostMessage(w http.ResponseWriter, r *http.Request, convID string) {
	var req postMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		encode(w, http.StatusBadRequest, nil, err, nil)
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
		ctx, cancel := context.WithCancel(context.Background())
		// Register cancel so external caller can abort.
		s.registerCancel(convID, cancel)
		defer func() {
			s.completeCancel(convID, cancel)
			cancel()
		}()
		// Carry conversation ID so downstream services (MCP client) can
		// persist interactive prompts even before the workflow explicitly
		// sets it again.
		// Propagate conversation ID via strongly typed context key so that
		// downstream services (Fluxor, MCP client, awaiters) can unambiguously
		// identify the conversation.
		ctx = conversation.WithID(ctx, convID)

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

// handleTerminateConversation processes POST /v1/api/conversations/{id}/terminate
// and attempts to cancel all in-flight turns for the given conversation. It is
// a best-effort operation – when no turn is running the endpoint still returns
// 204 so the client can treat the conversation as idle.
func (s *Server) handleTerminateConversation(w http.ResponseWriter, r *http.Request, convID string) {
	if r.Method != http.MethodPost {
		encode(w, http.StatusMethodNotAllowed, nil, fmt.Errorf("method not allowed"), nil)
		return
	}
	if strings.TrimSpace(convID) == "" {
		encode(w, http.StatusBadRequest, nil, fmt.Errorf("conversation id required"), nil)
		return
	}

	cancelled := s.cancelConversation(convID)

	status := http.StatusAccepted
	if !cancelled {
		status = http.StatusNoContent
	}

	encode(w, status, map[string]any{"cancelled": cancelled}, nil, nil)
}

// ListenAndServe Simple helper to start the server (blocks).
func ListenAndServe(addr string, mgr *conversation.Manager) error {
	handler := NewServer(mgr)
	log.Printf("HTTP chat server listening on %s", addr)
	return http.ListenAndServe(addr, handler)
}
