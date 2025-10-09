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
	"sync/atomic"
	"time"

	"errors"

	"github.com/viant/agently/adapter/http/ui"
	"github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/elicitation"
	elicrouter "github.com/viant/agently/genai/elicitation/router"
	"github.com/viant/agently/genai/memory"
	agconv "github.com/viant/agently/pkg/agently/conversation"

	"github.com/viant/agently/genai/conversation"
	execcfg "github.com/viant/agently/genai/executor/config"
	corellm "github.com/viant/agently/genai/service/core"
	"github.com/viant/agently/genai/tool"
	chat "github.com/viant/agently/internal/service/chat"
	"github.com/viant/agently/metadata"
	invk "github.com/viant/agently/pkg/agently/tool/invoker"
	"github.com/viant/datly"

	fluxpol "github.com/viant/fluxor/policy"
	"github.com/viant/fluxor/service/approval"

	"github.com/viant/agently/internal/auth"
	fservice "github.com/viant/forge/backend/service/file"
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
	fileSvc         *fservice.Service
	agentFinder     agent.Finder
	mcpRouter       elicrouter.ElicitationRouter

	// store removed; using conversation client via chat service

	invoker  invk.Invoker
	core     *corellm.Service
	defaults *execcfg.Defaults

	// Non-blocking compaction guard per conversation
	compactGuardMu sync.Mutex
	compactGuards  map[string]*int32

	// Optional auth + dao for token refresh
	authCfg *auth.Config
	dao     *datly.Service
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

// WithStore injects a domain.store so that v1 endpoints can read from DAO-backed store
// when AGENTLY_V1_DOMAIN=1 is set. When store is nil or the flag is not set, legacy memory
// reads remain in effect.
// WithStore removed; chat service no longer depends on domain.store

// WithApprovalService injects the Fluxor approval service so that the HTTP
// callback handler can forward Accept/Decline decisions to the workflow
// engine. Supplying the service is optional — when nil the server falls back
// to chat-only status updates.
func WithApprovalService(svc approval.Service) ServerOption {
	return func(s *Server) { s.pendingApproval = svc }
}

// WithFileService injects the Forge file service so chat service can reuse
// the same staging and storage resolution for reading uploaded attachments.
func WithFileService(fs *fservice.Service) ServerOption {
	return func(s *Server) {
		s.fileSvc = fs
	}
}

// WithAgentFinder returns with agent finder
func WithAgentFinder(finder agent.Finder) ServerOption {
	return func(s *Server) {
		s.agentFinder = finder
	}
}

// WithChatService injects a preconfigured chat service. When provided,
// NewServer will not attempt to auto-initialize chat from env.
func WithChatService(c *chat.Service) ServerOption {
	return func(s *Server) { s.chatSvc = c }
}

// WithInvoker attaches a tool invoker used by transcript extensions with source=invoke.
func WithInvoker(inv invk.Invoker) ServerOption { return func(s *Server) { s.invoker = inv } }

// WithMCPRouter attaches an elicitation router to route MCP prompts back to the correct conversation.
func WithMCPRouter(r elicrouter.ElicitationRouter) ServerOption {
	return func(s *Server) { s.mcpRouter = r }
}

// WithCore attaches the LLM core service so Chat can call core.Generate directly.
func WithCore(c *corellm.Service) ServerOption { return func(s *Server) { s.core = c } }

// WithDefaults passes summary defaults (model/prompt/lastN) to chat service.
func WithDefaults(d *execcfg.Defaults) ServerOption { return func(s *Server) { s.defaults = d } }

// WithAuthConfig passes auth configuration for BFF token refresh.
func WithAuthConfig(cfg *auth.Config) ServerOption { return func(s *Server) { s.authCfg = cfg } }

// WithDAO passes a shared datly service so the server can read user ids.
func WithDAO(dao *datly.Service) ServerOption { return func(s *Server) { s.dao = dao } }

// NewServer returns an http.Handler with routes bound.
func NewServer(mgr *conversation.Manager, opts ...ServerOption) http.Handler {
	s := &Server{mgr: mgr}
	s.compactGuards = make(map[string]*int32)
	for _, o := range opts {
		if o != nil {
			o(s)
		}
	}
	// Ensure elicitation router is always configured
	if s.mcpRouter == nil {
		s.mcpRouter = elicrouter.New()
	}
	// Initialize chat service when not injected by caller (legacy behaviour).
	if s.chatSvc == nil {
		s.chatSvc = chat.NewService()
	}

	if s.agentFinder != nil {
		s.chatSvc.AttacheAgentFinder(s.agentFinder)
	}

	s.chatSvc.AttachManager(mgr, s.toolPolicy, s.fluxPolicy)
	if s.pendingApproval != nil {
		s.chatSvc.AttachApproval(s.pendingApproval)
	}
	s.chatSvc.AttachFileService(s.fileSvc)
	if s.core != nil {
		s.chatSvc.AttachCore(s.core)
	}
	if s.defaults != nil {
		s.chatSvc.AttachDefaults(s.defaults)
	}

	// Attach a shared elicitation service for persistence and waiting
	if s.chatSvc != nil {
		es := elicitation.New(s.chatSvc.ConversationClient(), nil, s.mcpRouter, nil)
		s.chatSvc.AttachElicitationService(es)
	}
	mux := http.NewServeMux()

	// ------------------------------------------------------------------
	// Chat API (Go 1.22+ pattern based routing)
	// Register only when conversation client is configured; otherwise
	// skip endpoints to avoid nil dereferences at runtime.
	// ------------------------------------------------------------------
	if s.chatSvc != nil && s.chatSvc.ConversationClient() != nil {
		// Conversations collection
		mux.HandleFunc("POST /v1/api/conversations", s.handleConversations)     // create new conversation
		mux.HandleFunc("GET /v1/api/conversations", s.handleConversations)      // list conversations
		mux.HandleFunc("GET /v1/api/conversations/{id}", s.handleConversations) // get conversation by id

		// Delete conversation (cascades via DB FKs)
		mux.HandleFunc("DELETE /v1/api/conversations/{id}", func(w http.ResponseWriter, r *http.Request) {
			s.handleDeleteConversation(w, r, r.PathValue("id"))
		})

		// Conversation messages collection & item
		mux.HandleFunc("POST /v1/api/conversations/{id}/messages", func(w http.ResponseWriter, r *http.Request) {
			s.handlePostMessage(w, r, r.PathValue("id"))
		})

		mux.HandleFunc("GET /v1/api/conversations/{id}/messages", func(w http.ResponseWriter, r *http.Request) {
			s.handleGetMessages(w, r, r.PathValue("id"))
		})

		// Delete a message within a conversation
		mux.HandleFunc("DELETE /v1/api/conversations/{id}/messages/{msgId}", func(w http.ResponseWriter, r *http.Request) {
			convID := r.PathValue("id")
			msgID := r.PathValue("msgId")
			s.handleDeleteMessage(w, r, convID, msgID)
		})
	}

	// Usage statistics
	// Usage endpoint removed; usage is computed client-side from messages.

	// Terminate/cancel current turn – best-effort cancellation of running workflow.
	mux.HandleFunc("POST /v1/api/conversations/{id}/terminate", func(w http.ResponseWriter, r *http.Request) {
		s.handleTerminateConversation(w, r, r.PathValue("id"))
	})

	// Run MCP tool in the context of a conversation (enrich args with conv context)
	mux.HandleFunc("POST /v1/api/conversations/{id}/tools/run", func(w http.ResponseWriter, r *http.Request) {
		s.handleRunTool(w, r, r.PathValue("id"))
	})

	// Compact conversation: generate summary and flag prior messages as compacted
	mux.HandleFunc("POST /v1/api/conversations/{id}/compact", func(w http.ResponseWriter, r *http.Request) {
		s.handleCompactConversation(w, r, r.PathValue("id"))
	})

	// ------------------------------------------------------------------
	// Forge UI metadata endpoints
	// ------------------------------------------------------------------
	// Serve UI metadata from embedded YAML definitions.
	uiRoot := "embed://localhost/"
	uiHandler := ui.NewEmbeddedHandler(uiRoot, &metadata.FS)
	mux.Handle("/v1/api/agently/forge/", http.StripPrefix("/v1/api/agently/forge", uiHandler))

	// Conversation-scoped elicitation callback (preferred)
	mux.HandleFunc("POST /v1/api/conversations/{id}/elicitation/{elicId}", func(w http.ResponseWriter, r *http.Request) {
		convID := r.PathValue("id")
		elicID := r.PathValue("elicId")
		s.handleElicitationCallback(w, r, convID, elicID)
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

		input := &agconv.ConversationInput{}

		if component, _ := s.dao.Component(r.Context(), agconv.ConversationsPathURI); component != nil {
			if injector, _ := s.dao.GetInjector(r, component); injector != nil {
				injector.Bind(r.Context(), input)
				fmt.Printf("injected:  %+v\n", input)
			}
		}

		list, err := s.chatSvc.ListConversations(s.withAuthFromRequest(r), input)
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

	// First fetch (or only when extensions are not requested)
	baseCtx := r.Context()
	if s.invoker != nil {
		baseCtx = invk.With(baseCtx, s.invoker)
	}
	baseCtx = memory.WithConversationID(baseCtx, convID)

	// Resolve applicable extensions via chat service (moves heavy logic out of handler)
	toolSpec, err := s.chatSvc.MatchToolFeedSpec(baseCtx, convID, sinceId)
	if err != nil {
		encode(w, http.StatusInternalServerError, nil, err)
		return
	}
	// Second fetch with ToolExtensions populated so transcript hook can compute executions
	opts := chat.GetRequest{ConversationID: convID, SinceID: sinceId, IncludeModelCallPayload: includeModelCallPayload == "1", IncludeToolCall: true, ToolExtensions: toolSpec}
	conv, err := s.chatSvc.Get(baseCtx, opts)
	if err != nil {
		encode(w, http.StatusInternalServerError, nil, err)
		return
	}
	encode(w, http.StatusOK, conv.Conversation, nil)
}

// handleDeleteMessage deletes a specific message by id within a conversation.
func (s *Server) handleDeleteMessage(w http.ResponseWriter, r *http.Request, convID, msgID string) {
	if r.Method != http.MethodDelete {
		encode(w, http.StatusMethodNotAllowed, nil, fmt.Errorf("method not allowed"))
		return
	}
	if s.chatSvc == nil || s.chatSvc.ConversationClient() == nil {
		encode(w, http.StatusInternalServerError, nil, fmt.Errorf("chat service not initialised"))
		return
	}
	if err := s.chatSvc.DeleteMessage(r.Context(), convID, msgID); err != nil {
		encode(w, http.StatusInternalServerError, nil, err)
		return
	}
	encode(w, http.StatusOK, map[string]string{"id": msgID, "status": "deleted"}, nil)
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
	case "tools":
		// POST /v1/api/conversations/{id}/tools/run
		if len(parts) >= 3 && parts[2] == "run" {
			s.handleRunTool(w, r, convID)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// handleRunTool executes an MCP tool in the context of a conversation.
// POST body: { "service": "system/patch", "method": "commit", "args": { ... } }
func (s *Server) handleRunTool(w http.ResponseWriter, r *http.Request, convID string) {
	if r.Method != http.MethodPost {
		encode(w, http.StatusMethodNotAllowed, nil, fmt.Errorf("method not allowed"))
		return
	}
	if strings.TrimSpace(convID) == "" {
		encode(w, http.StatusBadRequest, nil, fmt.Errorf("conversation id required"))
		return
	}
	if s.invoker == nil {
		encode(w, http.StatusInternalServerError, nil, fmt.Errorf("tool invoker not configured"))
		return
	}
	var body struct {
		Service string                 `json:"service"`
		Method  string                 `json:"method"`
		Args    map[string]interface{} `json:"args"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		encode(w, http.StatusBadRequest, nil, err)
		return
	}
	if strings.TrimSpace(body.Service) == "" {
		encode(w, http.StatusBadRequest, nil, fmt.Errorf("service is required"))
		return
	}

	// Build context and enrich args via chat service helper
	ctx := s.withAuthFromRequest(r)
	if s.chatSvc == nil {
		encode(w, http.StatusInternalServerError, nil, fmt.Errorf("chat service not initialised"))
		return
	}
	var err error
	ctx, body.Args, err = s.chatSvc.PrepareToolContext(ctx, convID, body.Args)
	if err != nil {
		encode(w, http.StatusBadRequest, nil, err)
		return
	}

	// Invoke tool via invoker
	out, err := s.invoker.Invoke(ctx, body.Service, body.Method, body.Args)
	if err != nil {
		encode(w, http.StatusInternalServerError, nil, err)
		return
	}
	encode(w, http.StatusOK, out, nil)
}

// handleElicitationCallback accepts/declines an MCP elicitation for a specific conversation.
func (s *Server) handleElicitationCallback(w http.ResponseWriter, r *http.Request, convID, elicID string) {
	if r.Method != http.MethodPost {
		encode(w, http.StatusMethodNotAllowed, nil, fmt.Errorf("method not allowed"))
		return
	}
	if strings.TrimSpace(convID) == "" || strings.TrimSpace(elicID) == "" {
		encode(w, http.StatusBadRequest, nil, fmt.Errorf("conversation and elicitation id required"))
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

	if strings.TrimSpace(req.Action) == "" {
		encode(w, http.StatusBadRequest, nil, fmt.Errorf("action is required"))
		return
	}
	if s.chatSvc == nil {
		encode(w, http.StatusInternalServerError, nil, fmt.Errorf("chat service not initialised"))
		return
	}
	// Use elicitation service for status, payload and router notification
	eliciationService := s.chatSvc.ElicitationService()
	if eliciationService == nil {
		encode(w, http.StatusInternalServerError, nil, fmt.Errorf("elicitation service not initialised"))
		return
	}
	if err := eliciationService.Resolve(r.Context(), convID, elicID, req.Action, req.Payload); err != nil {
		// Map not found to 404; everything else bubbles as 500
		if strings.Contains(err.Error(), "elicitation message not found") {
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
	// Refresh access token for MCP/tools when BFF configured
	if s.authCfg != nil && s.authCfg.OAuth != nil && s.dao != nil {
		// derive user id from user service best-effort
		info := auth.User(ctx)
		uname := ""
		if info != nil {
			uname = strings.TrimSpace(info.Subject)
			if uname == "" {
				uname = strings.TrimSpace(info.Email)
			}
		}
		if uname != "" {
			if s.chatSvc != nil {
				if uid, err := s.chatSvc.UserByUsername(ctx, uname); err == nil && strings.TrimSpace(uid) != "" {
					store := auth.NewTokenStoreDAO(s.chatSvc.DAO(), s.authCfg.OAuth.Client.ConfigURL)
					prov := s.authCfg.OAuth.Name
					if strings.TrimSpace(prov) == "" {
						prov = "oauth"
					}
					if access, _, err := store.EnsureAccessToken(ctx, uid, prov, s.authCfg.OAuth.Client.ConfigURL); err == nil && strings.TrimSpace(access) != "" {
						ctx = auth.WithBearer(ctx, access)
					}
				}
			}
		}
	}
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
	if s.chatSvc == nil {
		encode(w, http.StatusInternalServerError, nil, fmt.Errorf("chat service not initialised"))
		return
	}
	cancelled = s.chatSvc.Cancel(convID)
	if err := s.chatSvc.SetLastAssistentMessageStatus(context.Background(), convID, "canceled"); err != nil {
		encode(w, http.StatusInternalServerError, nil, err)
		return
	}

	status := http.StatusAccepted
	if !cancelled {
		status = http.StatusNoContent
	}

	encode(w, status, map[string]any{"cancelled": cancelled}, nil)
}

// handleDeleteConversation processes DELETE /v1/api/conversations/{id}
// and removes the conversation; dependent rows are removed via DB FK cascade.
func (s *Server) handleDeleteConversation(w http.ResponseWriter, r *http.Request, convID string) {
	if r.Method != http.MethodDelete {
		encode(w, http.StatusMethodNotAllowed, nil, fmt.Errorf("method not allowed"))
		return
	}
	if strings.TrimSpace(convID) == "" {
		encode(w, http.StatusBadRequest, nil, fmt.Errorf("conversation id required"))
		return
	}
	if s.chatSvc == nil {
		encode(w, http.StatusInternalServerError, nil, fmt.Errorf("chat service not initialised"))
		return
	}
	if err := s.chatSvc.DeleteConversation(r.Context(), convID); err != nil {
		encode(w, http.StatusInternalServerError, nil, err)
		return
	}
	encode(w, http.StatusNoContent, nil, nil)
}

// handleCompactConversation processes POST /v1/api/conversations/{id}/compact
// and triggers server-side compaction: inserts an assistant summary message and
// flags prior messages as compacted. Returns 202 on success.
func (s *Server) handleCompactConversation(w http.ResponseWriter, r *http.Request, convID string) {
	if r.Method != http.MethodPost {
		encode(w, http.StatusMethodNotAllowed, nil, fmt.Errorf("method not allowed"))
		return
	}
	if strings.TrimSpace(convID) == "" {
		encode(w, http.StatusBadRequest, nil, fmt.Errorf("conversation id required"))
		return
	}
	if s.chatSvc == nil {
		encode(w, http.StatusInternalServerError, nil, fmt.Errorf("chat service not initialised"))
		return
	}
	// Non-blocking de-duplication via atomic guard per conversation.
	s.compactGuardMu.Lock()
	g := s.compactGuards[convID]
	if g == nil {
		var v int32
		g = &v
		s.compactGuards[convID] = g
	}
	s.compactGuardMu.Unlock()

	if !atomic.CompareAndSwapInt32(g, 0, 1) {
		// Another compaction in progress; treat as success (idempotent)
		encode(w, http.StatusAccepted, map[string]any{"compacted": true}, nil)
		return
	}
	defer atomic.StoreInt32(g, 0)

	// Preserve auth/user context (like POST message flow) so downstream can attribute userId
	ctx := s.withAuthFromRequest(r)
	// Debug: print user id derived from auth context
	if ui := auth.User(ctx); ui != nil {
		uname := strings.TrimSpace(ui.Subject)
		if uname == "" {
			uname = strings.TrimSpace(ui.Email)
		}
	}

	if err := s.chatSvc.Compact(ctx, convID); err != nil {
		encode(w, http.StatusInternalServerError, nil, err)
		return
	}
	encode(w, http.StatusAccepted, map[string]any{"compacted": true}, nil)
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
	// Start with existing request context to preserve middleware-provided identity
	ctx := r.Context()
	authz := r.Header.Get("Authorization")
	if strings.TrimSpace(authz) == "" {
		// If middleware provided identity (cookie session), keep it; else use anonymous
		if info := auth.User(ctx); info != nil && (strings.TrimSpace(info.Subject) != "" || strings.TrimSpace(info.Email) != "") {
			return ctx
		}
		return auth.WithUserInfo(ctx, &auth.UserInfo{Subject: "anonymous"})
	}
	token := auth.ExtractBearer(authz)
	if token == "" {
		if info := auth.User(ctx); info != nil && (strings.TrimSpace(info.Subject) != "" || strings.TrimSpace(info.Email) != "") {
			return ctx
		}
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
