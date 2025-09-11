package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/viant/agently/adapter/http/ui"
	mcpclient "github.com/viant/agently/adapter/mcp"
	"github.com/viant/agently/genai/conversation"
	"github.com/viant/agently/genai/memory"
	agentpkg "github.com/viant/agently/genai/service/agent"
	"github.com/viant/agently/genai/tool"
	convread "github.com/viant/agently/internal/dao/conversation/read"
	convw "github.com/viant/agently/internal/dao/conversation/write"
	"github.com/viant/agently/internal/dao/message/write"
	usageread "github.com/viant/agently/internal/dao/usage/read"
	"github.com/viant/agently/metadata"
	fluxpol "github.com/viant/fluxor/policy"
	"github.com/viant/fluxor/service/approval"
	"github.com/viant/mcp-protocol/schema"

	"github.com/google/uuid"
	plan "github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/stage"
	msgread "github.com/viant/agently/internal/dao/message/read"
	plread "github.com/viant/agently/internal/dao/payload/read"
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
	mgr         *conversation.Manager
	titles      sync.Map // convID -> title
	toolPolicy  *tool.Policy
	fluxPolicy  *fluxpol.Policy
	approvalSvc approval.Service

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

// WithDomainStore injects a domain.Store so that v1 endpoints can read from DAO-backed store
// when AGENTLY_V1_DOMAIN=1 is set. When store is nil or the flag is not set, legacy memory
// reads remain in effect.
func WithDomainStore(st d.Store) ServerOption { return func(s *Server) { s.store = st } }

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

	// Payload fetch (lazy request/response bodies)
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

		// Read optional overrides from request body into a typed struct
		type createConversationRequest struct {
			Model      string `json:"model"`
			Agent      string `json:"agent"`
			Tools      string `json:"tools"` // comma separated list
			Title      string `json:"title"`
			Visibility string `json:"visibility"`
		}
		var req createConversationRequest
		if r.Body != nil {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
				// tolerate invalid body for backward compatibility
				log.Printf("warn: ignoring invalid create conversation body: %v", err)
			}
		}
		if strings.TrimSpace(req.Title) != "" {
			title = strings.TrimSpace(req.Title)
		}
		createdAt := time.Now().UTC().Format(time.RFC3339)

		// Best-effort persist to domain store when available
		cw := &convw.Conversation{Has: &convw.ConversationHas{}}
		cw.SetId(id)
		cw.SetTitle(title)
		cw.SetCreatedAt(time.Now().UTC())
		// Default visibility is public unless explicitly provided
		if strings.TrimSpace(req.Visibility) == "" {
			cw.SetVisibility(convw.VisibilityPublic)
		} else {
			cw.SetVisibility(strings.TrimSpace(req.Visibility))
		}
		if strings.TrimSpace(req.Agent) != "" {
			cw.SetAgentName(strings.TrimSpace(req.Agent))
		}
		if strings.TrimSpace(req.Model) != "" {
			cw.SetDefaultModel(strings.TrimSpace(req.Model))
		}
		if strings.TrimSpace(req.Tools) != "" {
			parts := strings.Split(req.Tools, ",")
			tools := make([]string, 0, len(parts))
			for _, p := range parts {
				if s := strings.TrimSpace(p); s != "" {
					tools = append(tools, s)
				}
			}
			if len(tools) > 0 {
				meta := map[string]any{"tools": tools}
				if b, err := json.Marshal(meta); err == nil {
					cw.SetMetadata(string(b))
				}
			}
		}
		if _, err := s.store.Conversations().Patch(r.Context(), cw); err != nil {
			encode(w, http.StatusInternalServerError, nil, fmt.Errorf("failed to persist conversation: %w", err), nil)
			return
		}

		s.titles.Store(id, title)

		// No dependency on history store; rely on domain persistence only

		// Response mirrors request fields in a typed shape for stability
		type createConversationResponse struct {
			ID        string `json:"id"`
			Title     string `json:"title"`
			CreatedAt string `json:"createdAt"`
			Model     string `json:"model,omitempty"`
			Agent     string `json:"agent,omitempty"`
			Tools     string `json:"tools,omitempty"`
		}
		resp := createConversationResponse{ID: id, Title: title, CreatedAt: createdAt, Model: req.Model, Agent: req.Agent, Tools: req.Tools}
		encode(w, http.StatusOK, resp, nil, nil)
	case http.MethodGet:
		id := r.PathValue("id")
		// Prefer DAO listing to avoid reliance on history store
		// Single conversation fetch by id
		if strings.TrimSpace(id) != "" {
			cv, err := s.store.Conversations().Get(r.Context(), id)
			if err != nil {
				encode(w, http.StatusInternalServerError, nil, err, nil)
				return
			}
			if cv == nil {
				encode(w, http.StatusNotFound, nil, fmt.Errorf("conversation not found"), nil)
				return
			}
			t := id
			if cv.Title != nil && strings.TrimSpace(*cv.Title) != "" {
				t = *cv.Title
			}
			encode(w, http.StatusOK, []conversationInfo{{ID: id, Title: t}}, nil, nil)
			return
		}
		// List all conversations
		// Use an always-true archived filter to avoid predicate builder generating AND 1=0
		rows, err := s.store.Conversations().List(r.Context(), convread.WithArchived(0, 1))
		if err != nil {
			encode(w, http.StatusInternalServerError, nil, err, nil)
			return
		}
		conversations := make([]conversationInfo, 0, len(rows))
		for _, v := range rows {
			if v == nil {
				continue
			}
			t := v.Id
			if v.Title != nil && strings.TrimSpace(*v.Title) != "" {
				t = *v.Title
			}
			conversations = append(conversations, conversationInfo{ID: v.Id, Title: t})
		}
		encode(w, http.StatusOK, conversations, nil, nil)
		return
		// Fallback empty list when DAO not available
		encode(w, http.StatusOK, []conversationInfo{}, nil, nil)
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
	// Some clients append synthetic suffixes (e.g. "/form") to message IDs for UI-only entries.
	// Make since tolerant by stripping any suffix after the first '/'.
	if idx := strings.IndexByte(sinceId, '/'); idx > 0 {
		sinceId = sinceId[:idx]
	}
	opts := []msgread.InputOption{
		msgread.WithInterim(0),
		msgread.WithConversationID(convID),
	}
	if sinceId != "" {
		opts = append(opts, msgread.WithSinceID(sinceId))
	}

	// Domain-backed transcript with per-message tool outcomes
	views, err := s.store.Messages().GetTranscript(r.Context(), convID, opts...)
	if err != nil {
		encode(w, http.StatusInternalServerError, nil, err, s.currentStage(convID))
		return
	}

	msgs := make([]memory.Message, 0, len(views))
	for _, v := range views {
		if v == nil || v.IsInterim() {
			continue
		}
		mm := memory.Message{ID: v.Id, ConversationID: v.ConversationID, Role: v.Role, Content: v.Content}
		if v.ParentID != nil {
			mm.ParentID = *v.ParentID
		}
		if v.ToolName != nil {
			mm.ToolName = v.ToolName
		}
		if v.CreatedAt != nil {
			mm.CreatedAt = *v.CreatedAt
		} else {
			mm.CreatedAt = time.Now()
		}
		if v.Elicitation != nil {
			mm.Elicitation = v.Elicitation
		}
		// Build single-step outcome for tool messages
		if strings.EqualFold(strings.TrimSpace(v.Role), "tool") && v.ToolCall != nil {
			st := &plan.StepOutcome{
				ID:                v.ToolCall.OpID,
				Name:              v.ToolCall.ToolName,
				Reason:            v.Content,
				Success:           strings.EqualFold(strings.TrimSpace(v.ToolCall.Status), "completed"),
				Error:             derefStr(v.ToolCall.ErrorMessage),
				StartedAt:         v.ToolCall.StartedAt,
				EndedAt:           v.ToolCall.CompletedAt,
				RequestPayloadID:  v.ToolCall.RequestPayloadID,
				ResponsePayloadID: v.ToolCall.ResponsePayloadID,
			}
			if v.ToolCall.StartedAt != nil && v.ToolCall.CompletedAt != nil {
				st.Elapsed = v.ToolCall.CompletedAt.Sub(*v.ToolCall.StartedAt).Round(time.Millisecond).String()
			}
			// Inline request/response bodies if available
			if v.ToolCall.RequestPayloadID != nil && *v.ToolCall.RequestPayloadID != "" {
				if pv, e := s.store.Payloads().Get(r.Context(), *v.ToolCall.RequestPayloadID); e == nil && pv != nil && pv.InlineBody != nil {
					st.Request = json.RawMessage(*pv.InlineBody)
				} else if e != nil {
					encode(w, http.StatusInternalServerError, nil, e, s.currentStage(convID))
					return
				}
			}
			if v.ToolCall.ResponsePayloadID != nil && *v.ToolCall.ResponsePayloadID != "" {
				if pv, e := s.store.Payloads().Get(r.Context(), *v.ToolCall.ResponsePayloadID); e == nil && pv != nil && pv.InlineBody != nil {
					st.Response = json.RawMessage(*pv.InlineBody)
				} else if e != nil {
					encode(w, http.StatusInternalServerError, nil, e, s.currentStage(convID))
					return
				}
			}
			mm.Executions = []*plan.Outcome{{Steps: []*plan.StepOutcome{st}}}
		}
		msgs = append(msgs, mm)
	}

	if sinceId != "" && len(msgs) == 0 {
		encode(w, http.StatusProcessing, msgs, nil, s.currentStage(convID))
		return
	}
	encode(w, http.StatusOK, msgs, nil, s.currentStage(convID))
}

func derefStr(p *string) string {
	if p != nil {
		return *p
	}
	return ""
}

// currentStage infers live phase of a conversation based on recent transcript.
// Heuristics:
// - waiting: no messages
// - executing: latest tool call present and running (completed_at nil or status==running)
// - elicitation: latest assistant message carries elicitation request
// - thinking: last message is user (no assistant response yet)
// - error: latest tool call status failed and no newer assistant success
// - done: otherwise
func (s *Server) currentStage(convID string) *stage.Stage {
	st := &stage.Stage{Phase: stage.StageWaiting}
	if s == nil || s.store == nil || strings.TrimSpace(convID) == "" {
		return st
	}
	views, err := s.store.Messages().GetTranscript(context.Background(), convID)
	if err != nil || len(views) == 0 {
		return st
	}
	// Work from the end to detect freshest signals
	lastRole := ""
	lastAssistantElic := false
	lastToolStatus := ""
	lastToolRunning := false
	lastToolFailed := false
	lastModelRunning := false
	for i := len(views) - 1; i >= 0; i-- {
		v := views[i]
		if v == nil || v.IsInterim() {
			continue
		}
		r := strings.ToLower(strings.TrimSpace(v.Role))
		if lastRole == "" {
			lastRole = r
		}
		// Tool signals
		if v.ToolCall != nil {
			status := strings.ToLower(strings.TrimSpace(v.ToolCall.Status))
			lastToolStatus = status
			if status == "running" || v.ToolCall.CompletedAt == nil {
				lastToolRunning = true
				break
			}
			if status == "failed" {
				lastToolFailed = true
			}
		}
		// Model call signals (thinking)
		if v.ModelCall != nil {
			mstatus := strings.ToLower(strings.TrimSpace(v.ModelCall.Status))
			if mstatus == "running" || v.ModelCall.CompletedAt == nil {
				lastModelRunning = true
				break
			}
		}
		// Assistant elicitation
		if r == "assistant" && v.Elicitation != nil {
			lastAssistantElic = true
			break
		}
		// Stop at first non-interim meaningful signal
		if r == "assistant" || r == "user" || r == "tool" {
			// keep scanning for tool or elicitation if needed
		}
	}

	switch {
	case lastToolRunning:
		st.Phase = stage.StageExecuting
	case lastAssistantElic:
		st.Phase = stage.StageEliciting
	case lastModelRunning:
		st.Phase = stage.StageThinking
	case lastRole == "user":
		st.Phase = stage.StageThinking
	case lastToolFailed:
		st.Phase = stage.StageError
	default:
		st.Phase = stage.StageDone
	}
	_ = lastToolStatus // reserved for future tool name/reporting
	return st
}

// handleGetSingleMessage supports GET /v1/api/conversations/{id}/messages/{msgID}
func (s *Server) handleGetSingleMessage(w http.ResponseWriter, r *http.Request, convID, msgID string) {
	if msgID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	// Domain-backed lookup when store is available
	views, err := s.store.Messages().List(r.Context(), msgread.WithIDs(msgID))
	if err != nil {
		encode(w, http.StatusInternalServerError, nil, err, nil)
		return
	}
	if len(views) == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	v := views[0]
	mm := memory.Message{ID: v.Id, ConversationID: v.ConversationID, Role: v.Role, Content: v.Content}
	if v.CreatedAt != nil {
		mm.CreatedAt = *v.CreatedAt
	}
	if v.ToolName != nil {
		mm.ToolName = v.ToolName
	}
	if v.ElicitationID != nil {
		if pv, err := s.store.Payloads().List(r.Context(), plread.WithID(*v.ElicitationID)); err == nil && len(pv) > 0 && pv[0] != nil && pv[0].InlineBody != nil {
			var e plan.Elicitation
			if json.Unmarshal(*pv[0].InlineBody, &e) == nil {
				mm.Elicitation = &e
			}
		}
	}
	encode(w, http.StatusOK, mm, nil, nil)
	return
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
	if id == "" || s.store == nil {
		encode(w, http.StatusNotFound, nil, fmt.Errorf("payload not found"), nil)
		return
	}
	rows, err := s.store.Payloads().List(r.Context(), plread.WithID(id))
	if err != nil {
		encode(w, http.StatusInternalServerError, nil, err, nil)
		return
	}
	if len(rows) == 0 || rows[0] == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	v := rows[0]
	q := strings.ToLower(r.URL.Query().Get("raw"))
	if q == "1" || q == "true" || q == "yes" {
		if v.InlineBody == nil || len(*v.InlineBody) == 0 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if strings.TrimSpace(v.MimeType) != "" {
			w.Header().Set("Content-Type", v.MimeType)
		} else {
			w.Header().Set("Content-Type", "application/octet-stream")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(*v.InlineBody)
		return
	}
	// meta or inline JSON envelope
	meta := strings.ToLower(r.URL.Query().Get("meta"))
	copy := *v
	if meta == "1" || meta == "true" || meta == "yes" {
		copy.InlineBody = nil
	}
	encode(w, http.StatusOK, &copy, nil, nil)
}

// payloadIDFromSnapshot extracts payloadId from a snapshot JSON string.
func payloadIDFromSnapshot(snapshot *string) string {
	if snapshot == nil || *snapshot == "" {
		return ""
	}
	var x struct {
		PayloadID string `json:"payloadId"`
	}
	if json.Unmarshal([]byte(*snapshot), &x) == nil {
		return x.PayloadID
	}
	return ""
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

// handleGetUsage responds with aggregated + per-model token usage.
func (s *Server) handleGetUsage(w http.ResponseWriter, r *http.Request, convID string) {
	// Require GET only.
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Domain-backed path: map DAO usage rows to legacy usage payload
	in := usageread.Input{ConversationID: convID, Has: &usageread.Has{ConversationID: true}}
	rows, err := s.store.Usage().List(r.Context(), in)
	if err != nil {
		encode(w, http.StatusInternalServerError, nil, err, nil)
		return
	}
	totals := usagePayload{ConversationID: convID, PerModel: []usagePerModel{}}
	for _, v := range rows {
		if v == nil {
			continue
		}
		pm := usagePerModel{Model: strings.TrimSpace(v.Provider + "/" + v.Model)}
		if v.TotalPromptTokens != nil {
			pm.InputTokens = *v.TotalPromptTokens
			totals.InputTokens += *v.TotalPromptTokens
		}
		if v.TotalCompletionTokens != nil {
			pm.OutputTokens = *v.TotalCompletionTokens
			totals.OutputTokens += *v.TotalCompletionTokens
		}
		// Fallback: when provider reports only total_tokens, attribute them to output
		if (pm.InputTokens+pm.OutputTokens) == 0 && v.TotalTokens != nil {
			pm.OutputTokens += *v.TotalTokens
			totals.OutputTokens += *v.TotalTokens
		}
		// No embedding/cached in DAO row → keep zero; TotalTokens computed at end
		totals.PerModel = append(totals.PerModel, pm)
	}
	// Stable ordering for UI diffing
	sort.SliceStable(totals.PerModel, func(i, j int) bool { return totals.PerModel[i].Model < totals.PerModel[j].Model })
	totals.TotalTokens = totals.InputTokens + totals.OutputTokens + totals.EmbeddingTokens + totals.CachedTokens
	encode(w, http.StatusOK, []usagePayload{totals}, nil, nil)
	return
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

	messages, _ := s.store.Messages().List(r.Context(), msgread.WithIDs(messageID))
	if len(messages) == 0 {
		encode(w, http.StatusNotFound, nil, fmt.Errorf("interaction message not found"), nil)
		return
	}
	message := messages[0]

	// Find the conversation containing this message.

	// Update message status.
	status := "declined"
	if req.Action == "accept" {
		status = "done"
	}

	s.store.Messages().Patch(r.Context(), &write.Message{
		Id:     message.Id,
		Status: status,
		Has: &write.MessageHas{
			Status: true,
		},
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

	messages, _ := s.store.Messages().List(r.Context(), msgread.WithIDs(messageId))
	if len(messages) == 0 {
		encode(w, http.StatusNotFound, nil, fmt.Errorf("interaction message not found"), nil)
		return
	}
	message := messages[0]

	// 1) Persist decision in Conversation history so UI updates immediately.
	newStatus := "declined"
	if approved {
		newStatus = "done"
	}

	s.store.Messages().Patch(r.Context(), &write.Message{
		Id:     message.Id,
		Status: newStatus,
		Has: &write.MessageHas{
			Status: true,
		},
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
