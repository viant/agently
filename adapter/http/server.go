package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
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
	"github.com/viant/agently/genai/modelcallctx"
	"github.com/viant/agently/genai/streaming"
	agconv "github.com/viant/agently/pkg/agently/conversation"

	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/conversation"
	execcfg "github.com/viant/agently/genai/executor/config"
	corellm "github.com/viant/agently/genai/service/core"
	"github.com/viant/agently/genai/tool"
	chat "github.com/viant/agently/internal/service/chat"
	"github.com/viant/agently/metadata"
	invk "github.com/viant/agently/pkg/agently/tool/invoker"
	"github.com/viant/datly"

	approval "github.com/viant/agently/internal/approval"

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
	// Non-blocking prune guard per conversation
	pruneGuardMu sync.Mutex
	pruneGuards  map[string]*int32

	// Optional auth + dao for token refresh
	authCfg *auth.Config
	dao     *datly.Service

	eventSeqMu sync.Mutex
	eventSeq   map[string]uint64
	streamPub  *streaming.Publisher
}

// ServerOption customises HTTP server behaviour.
type ServerOption func(*Server)

// WithExecutionStore attaches an in-memory ExecutionTrace store so that GET
// /v1/api/conversations/{id}/tool-trace can return audit information.
// WithExecutionStore removed; execution traces now reconstructed from DAO tool_calls when needed.

// WithPolicies injects default tool policies so that API requests inherit
// the configured mode (auto/ask/deny).
func WithPolicies(tp *tool.Policy) ServerOption {
	return func(s *Server) {
		s.toolPolicy = tp
	}
}

// WithStore injects a domain.store so that v1 endpoints can read from DAO-backed store
// when AGENTLY_V1_DOMAIN=1 is set. When store is nil or the flag is not set, legacy memory
// reads remain in effect.
// WithStore removed; chat service no longer depends on domain.store

// WithApprovalService injects an approval service used by the HTTP callback
// handler to forward Accept/Decline decisions. Optional; when nil the server
// falls back to chat-only status updates.
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
	s.pruneGuards = make(map[string]*int32)
	s.eventSeq = make(map[string]uint64)
	s.streamPub = streaming.NewPublisher()
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

	s.chatSvc.AttachManager(mgr, s.toolPolicy)
	if s.pendingApproval != nil {
		s.chatSvc.AttachApproval(s.pendingApproval)
	}
	s.chatSvc.AttachFileService(s.fileSvc)
	if s.core != nil {
		s.chatSvc.AttachCore(s.core)
		s.core.SetStreamPublisher(s.streamPub)
	}
	if s.defaults != nil {
		s.chatSvc.AttachDefaults(s.defaults)
	}
	if s.authCfg != nil {
		s.chatSvc.AttachAuthConfig(s.authCfg)
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
		mux.HandleFunc("PATCH /v1/api/conversations/{id}", s.handleConversations)

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
		mux.HandleFunc("GET /v1/api/conversations/{id}/events", func(w http.ResponseWriter, r *http.Request) {
			s.handleConversationEvents(w, r, r.PathValue("id"))
		})

		// Delete a message within a conversation
		mux.HandleFunc("DELETE /v1/api/conversations/{id}/messages/{msgId}", func(w http.ResponseWriter, r *http.Request) {
			convID := r.PathValue("id")
			msgID := r.PathValue("msgId")
			s.handleDeleteMessage(w, r, convID, msgID)
		})

		// Cancel a queued turn within a conversation
		mux.HandleFunc("DELETE /v1/api/conversations/{id}/turns/{turnId}", func(w http.ResponseWriter, r *http.Request) {
			convID := r.PathValue("id")
			turnID := r.PathValue("turnId")
			s.handleDeleteTurn(w, r, convID, turnID)
		})

		// Reorder a queued turn within a conversation (swap with neighbor)
		mux.HandleFunc("POST /v1/api/conversations/{id}/turns/{turnId}/move", func(w http.ResponseWriter, r *http.Request) {
			convID := r.PathValue("id")
			turnID := r.PathValue("turnId")
			s.handleMoveQueuedTurn(w, r, convID, turnID)
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
	// Prune conversation: remove low-value messages via LLM-guided pruning
	mux.HandleFunc("POST /v1/api/conversations/{id}/prune", func(w http.ResponseWriter, r *http.Request) {
		s.handlePruneConversation(w, r, r.PathValue("id"))
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

func parsePageSize(r *http.Request) (int, int, bool) {
	if r == nil {
		return 0, 0, false
	}
	q := r.URL.Query()
	pageStr := strings.TrimSpace(q.Get("page"))
	sizeStr := strings.TrimSpace(q.Get("size"))
	if pageStr == "" && sizeStr == "" {
		return 0, 0, false
	}
	page := 1
	if pageStr != "" {
		n, err := strconv.Atoi(pageStr)
		if err != nil || n < 1 {
			return 0, 0, false
		}
		page = n
	}
	size := 20
	if sizeStr != "" {
		n, err := strconv.Atoi(sizeStr)
		if err != nil || n < 1 {
			return 0, 0, false
		}
		size = n
	}
	return page, size, true
}

func paginateConversations(list []chat.ConversationSummary, page, size int) []chat.ConversationSummary {
	if size <= 0 || page < 1 {
		return list
	}
	start := (page - 1) * size
	if start >= len(list) {
		return []chat.ConversationSummary{}
	}
	end := start + size
	if end > len(list) {
		end = len(list)
	}
	return list[start:end]
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
			cv, err := s.chatSvc.GetConversation(s.withAuthFromRequest(r), id)
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

		if s.dao != nil {
			if component, _ := s.dao.Component(r.Context(), agconv.ConversationsPathURI); component != nil {
				if injector, _ := s.dao.GetInjector(r, component); injector != nil {
					injector.Bind(r.Context(), input)
				}
			}
		}

		list, err := s.chatSvc.ListConversations(s.withAuthFromRequest(r), input)
		if err != nil {
			encode(w, http.StatusInternalServerError, nil, err)
			return
		}
		q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
		// Optional: create a conversation when agentId is provided and no match found.
		if len(list) == 0 && q == "" {
			query := r.URL.Query()
			createIfMissing := strings.TrimSpace(query.Get("createIfMissing"))
			agentId := strings.TrimSpace(query.Get("agentId"))
			if (createIfMissing == "1" || strings.EqualFold(createIfMissing, "true")) && agentId != "" {
				// Create with minimal fields; Chat service enforces visibility/owner.
				if resp, cErr := s.chatSvc.CreateConversation(s.withAuthFromRequest(r), chat.CreateConversationRequest{Agent: agentId}); cErr == nil && resp != nil {
					createdAt := time.Now().UTC()
					if ts := strings.TrimSpace(resp.CreatedAt); ts != "" {
						if parsed, pErr := time.Parse(time.RFC3339Nano, ts); pErr == nil {
							createdAt = parsed
						}
					}
					list = append(list, chat.ConversationSummary{ID: resp.ID, Title: resp.Title, Summary: nil, CreatedAt: createdAt})
				}
			}
		}
		// Optional: simple search filter (title/summary/agent/model/id/tools)
		if q != "" && len(list) > 0 {
			filtered := make([]chat.ConversationSummary, 0, len(list))
			for _, c := range list {
				hay := strings.ToLower(strings.Join([]string{
					strings.TrimSpace(c.ID),
					strings.TrimSpace(c.Title),
					strings.TrimSpace(c.Agent),
					strings.TrimSpace(c.Model),
					func() string {
						if c.Summary != nil {
							return strings.TrimSpace(*c.Summary)
						}
						return ""
					}(),
					strings.Join(c.Tools, " "),
				}, " "))
				if strings.Contains(hay, q) {
					filtered = append(filtered, c)
				}
			}
			list = filtered
		}
		if page, size, ok := parsePageSize(r); ok {
			totalCount := len(list)
			list = paginateConversations(list, page, size)
			pageCount := 1
			if size > 0 {
				pageCount = (totalCount + size - 1) / size
				if pageCount < 1 {
					pageCount = 1
				}
			}
			resp := struct {
				Status string `json:"status"`
				Data   any    `json:"data"`
				Info   any    `json:"info,omitempty"`
			}{
				Status: "ok",
				Data:   list,
				Info: map[string]any{
					"pageCount":  pageCount,
					"totalCount": totalCount,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		encode(w, http.StatusOK, list, nil)
	case http.MethodPatch:
		id := strings.TrimSpace(r.PathValue("id"))
		if id == "" {
			encode(w, http.StatusBadRequest, nil, fmt.Errorf("conversation id is required"))
			return
		}
		var req chat.PatchConversationRequest
		if r.Body != nil {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
				encode(w, http.StatusBadRequest, nil, err)
				return
			}
		}
		if err := s.chatSvc.PatchConversation(s.withAuthFromRequest(r), id, req); err != nil {
			encode(w, http.StatusBadRequest, nil, err)
			return
		}
		encode(w, http.StatusOK, map[string]string{"id": id}, nil)
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
	includeLinked := strings.TrimSpace(r.URL.Query().Get("includeLinked")) == "1"

	// First fetch (or only when extensions are not requested)
	baseCtx := s.withAuthFromRequest(r)
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
	if conv == nil || conv.Conversation == nil {
		encode(w, http.StatusNotFound, nil, fmt.Errorf("conversation not found"))
		return
	}
	if !includeLinked {
		encode(w, http.StatusOK, sanitizeConversationForAPI(conv.Conversation), nil)
		return
	}

	// When requested, include child conversations referenced by link messages in this transcript.
	linked := map[string]*apiconv.Conversation{}
	if cc := s.chatSvc.ConversationClient(); cc != nil {
		if tr := conv.Conversation.GetTranscript(); tr != nil {
			seen := map[string]struct{}{}
			for _, turn := range tr {
				if turn == nil {
					continue
				}
				for _, m := range turn.GetMessages() {
					if m == nil || m.LinkedConversationId == nil {
						continue
					}
					id := strings.TrimSpace(*m.LinkedConversationId)
					if id == "" {
						continue
					}
					if _, ok := seen[id]; ok {
						continue
					}
					if child, err := cc.GetConversation(baseCtx, id); err == nil && child != nil {
						linked[id] = child
						seen[id] = struct{}{}
					}
				}
			}
		}
	}
	// Wrap response to preserve backward compatibility when includeLinked=1 is not passed.
	resp := struct {
		Conversation *apiconv.Conversation            `json:"conversation"`
		Linked       map[string]*apiconv.Conversation `json:"linked,omitempty"`
	}{Conversation: sanitizeConversationForAPI(conv.Conversation), Linked: linked}
	encode(w, http.StatusOK, resp, nil)
}

type streamMessageEnvelope struct {
	Seq            uint64              `json:"seq"`
	Time           time.Time           `json:"time"`
	ConversationID string              `json:"conversationId"`
	Message        *agconv.MessageView `json:"message"`
	ContentType    string              `json:"contentType,omitempty"`
	Content        interface{}         `json:"content,omitempty"`
}

// handleConversationEvents streams conversation messages over SSE.
// Query params:
//   - since=<messageId> (default uses Last-Event-ID)
//   - include=text,tool_op,control (optional filter by message type)
//   - history=1 (include existing messages when since is empty)
//   - includeModelCallPayload=1 (include model call payloads in message view)
func (s *Server) handleConversationEvents(w http.ResponseWriter, r *http.Request, convID string) {
	if r.Method != http.MethodGet {
		encode(w, http.StatusMethodNotAllowed, nil, fmt.Errorf("method not allowed"))
		return
	}
	if s.chatSvc == nil {
		encode(w, http.StatusInternalServerError, nil, fmt.Errorf("chat service not initialised"))
		return
	}
	includeTypes := parseIncludeTypes(r.URL.Query().Get("include"))
	includeHistory := strings.TrimSpace(r.URL.Query().Get("history")) == "1"
	includeModelCallPayload := strings.TrimSpace(r.URL.Query().Get("includeModelCallPayload")) == "1"
	lastSeenRaw := strings.TrimSpace(r.URL.Query().Get("since"))
	if lastSeenRaw == "" {
		lastSeenRaw = strings.TrimSpace(r.Header.Get("Last-Event-ID"))
	}
	lastSeenSeq, hasSeq := parseEventSeq(lastSeenRaw)
	lastSeenID := ""
	if !hasSeq && lastSeenRaw != "" {
		lastSeenID = lastSeenRaw
	}

	baseCtx := s.withAuthFromRequest(r)
	if s.invoker != nil {
		baseCtx = invk.With(baseCtx, s.invoker)
	}
	baseCtx = memory.WithConversationID(baseCtx, convID)

	waitMs := parseWaitMS(r.URL.Query().Get("wait"), 0)
	if waitMs > 0 {
		s.handleConversationEventsPoll(w, r, baseCtx, convID, includeTypes, includeHistory, includeModelCallPayload, lastSeenSeq, hasSeq, lastSeenID, waitMs)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		encode(w, http.StatusInternalServerError, nil, fmt.Errorf("streaming unsupported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	sent := map[uint64]struct{}{}
	if hasSeq {
		sent[lastSeenSeq] = struct{}{}
	}
	if !includeHistory && !hasSeq && lastSeenID == "" {
		if seq := s.findLastMessageSeq(baseCtx, convID, includeModelCallPayload); seq > 0 {
			lastSeenSeq = seq
			hasSeq = true
			sent[seq] = struct{}{}
		} else if id := s.findLastMessageID(baseCtx, convID, includeModelCallPayload); id != "" {
			lastSeenID = id
		}
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	heartbeat := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	defer heartbeat.Stop()
	var deltaCh <-chan *modelcallctx.StreamEvent
	var deltaCancel func()
	if s.streamPub != nil {
		deltaCh, deltaCancel = s.streamPub.Subscribe(convID)
	}
	if deltaCancel != nil {
		defer deltaCancel()
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-deltaCh:
			if ev == nil {
				continue
			}
			env := &streamMessageEnvelope{
				Seq:            0,
				Time:           time.Now().UTC(),
				ConversationID: convID,
				Message:        sanitizeMessageForStream(ev.Message),
				ContentType:    ev.ContentType,
				Content:        ev.Content,
			}
			if err := writeSSEEvent(w, flusher, "interim_message", "", env); err != nil {
				return
			}
		case <-heartbeat.C:
			_, _ = io.WriteString(w, ": ping\n\n")
			flusher.Flush()
		case <-ticker.C:
			conv, err := s.chatSvc.Get(baseCtx, chat.GetRequest{
				ConversationID:          convID,
				SinceID:                 lastSeenID,
				IncludeModelCallPayload: includeModelCallPayload,
				IncludeToolCall:         true,
			})
			if err != nil {
				_ = writeSSEEvent(w, flusher, "error", "", map[string]string{"error": err.Error()})
				return
			}
			if conv == nil || conv.Conversation == nil {
				_ = writeSSEEvent(w, flusher, "error", "", map[string]string{"error": "conversation not found"})
				return
			}
			messages := flattenTranscriptMessages(conv.Conversation)
			start := 0
			if !hasSeq && lastSeenID != "" {
				for i, m := range messages {
					if m == nil {
						continue
					}
					if m.Id == lastSeenID {
						start = i + 1
						break
					}
				}
			}
			for i := start; i < len(messages); i++ {
				m := messages[i]
				if m == nil || strings.TrimSpace(m.Id) == "" {
					continue
				}
				seq := s.nextEventSeq(convID, m)
				if hasSeq && seq <= lastSeenSeq {
					continue
				}
				if len(includeTypes) > 0 {
					msgType := strings.ToLower(strings.TrimSpace(m.Type))
					if _, ok := includeTypes[msgType]; !ok {
						if _, ok := includeTypes["elicitation"]; !ok || !isElicitationMessageView(m) {
							continue
						}
					}
				}
				if _, ok := sent[seq]; ok {
					continue
				}
				env := &streamMessageEnvelope{
					Seq:            seq,
					Time:           time.Now().UTC(),
					ConversationID: convID,
					Message:        sanitizeMessageForStream(m),
				}
				if ctype, content := buildStreamContent(m); content != nil {
					env.ContentType = ctype
					env.Content = content
				}
				eventName := eventNameForMessage(env.Message, env.ContentType)
				if err := writeSSEEvent(w, flusher, eventName, strconv.FormatUint(seq, 10), env); err != nil {
					return
				}
				lastSeenSeq = seq
				hasSeq = true
				sent[seq] = struct{}{}
				lastSeenID = m.Id
			}
		}
	}
}

func (s *Server) handleConversationEventsPoll(w http.ResponseWriter, r *http.Request, baseCtx context.Context, convID string, includeTypes map[string]struct{}, includeHistory, includeModelCallPayload bool, lastSeenSeq uint64, hasSeq bool, lastSeenID string, waitMs int) {
	deadline := time.Now().Add(time.Duration(waitMs) * time.Millisecond)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	writeJSON := func(status int, payload interface{}) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(payload)
	}

	for {
		if r.Context().Err() != nil {
			return
		}
		envs, seq, lastID, err := s.collectEventEnvelopes(baseCtx, convID, includeTypes, includeHistory, includeModelCallPayload, lastSeenSeq, hasSeq, lastSeenID)
		if err != nil {
			encode(w, http.StatusInternalServerError, nil, err)
			return
		}
		if len(envs) > 0 {
			resp := struct {
				Events []*streamMessageEnvelope `json:"events"`
				Since  string                   `json:"since,omitempty"`
			}{Events: envs}
			if seq > 0 {
				resp.Since = strconv.FormatUint(seq, 10)
			}
			writeJSON(http.StatusOK, resp)
			return
		}
		if time.Now().After(deadline) {
			writeJSON(http.StatusOK, struct {
				Events []*streamMessageEnvelope `json:"events"`
			}{Events: nil})
			return
		}
		select {
		case <-ticker.C:
		case <-r.Context().Done():
			return
		}
		lastSeenSeq = seq
		lastSeenID = lastID
	}
}

func parseIncludeTypes(raw string) map[string]struct{} {
	items := strings.Split(raw, ",")
	out := map[string]struct{}{}
	for _, item := range items {
		t := strings.ToLower(strings.TrimSpace(item))
		if t == "" {
			continue
		}
		out[t] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func isStreamingInterimMessage(m *agconv.MessageView) bool {
	if m == nil {
		return false
	}
	if m.Interim == 0 {
		return false
	}
	if strings.ToLower(strings.TrimSpace(m.Role)) != "assistant" {
		return false
	}
	if strings.ToLower(strings.TrimSpace(m.Type)) != "text" {
		return false
	}
	return true
}

func parseWaitMS(raw string, def int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return def
	}
	return v
}

func parseEventSeq(raw string) (uint64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	v, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func writeSSEEvent(w io.Writer, flusher http.Flusher, eventName, eventID string, payload interface{}) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if eventID != "" {
		_, _ = fmt.Fprintf(w, "id: %s\n", eventID)
	}
	if eventName != "" {
		_, _ = fmt.Fprintf(w, "event: %s\n", eventName)
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
	flusher.Flush()
	return nil
}

func flattenTranscriptMessages(conv *apiconv.Conversation) []*agconv.MessageView {
	if conv == nil || conv.Transcript == nil {
		return nil
	}
	var out []*agconv.MessageView
	for _, turn := range conv.Transcript {
		if turn == nil || turn.Message == nil {
			continue
		}
		for _, m := range turn.Message {
			if m != nil && !isSummaryMessageView(m) {
				out = append(out, m)
			}
		}
	}
	return out
}

func buildTurnElicitationIndex(messages []*agconv.MessageView) map[string]struct{} {
	if len(messages) == 0 {
		return nil
	}
	out := map[string]struct{}{}
	for _, m := range messages {
		if m == nil || !isElicitationMessageView(m) {
			continue
		}
		if m.TurnId == nil {
			continue
		}
		turnID := strings.TrimSpace(*m.TurnId)
		if turnID == "" {
			continue
		}
		out[turnID] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func shouldSuppressAssistantMessage(m *agconv.MessageView, turnElicitation map[string]struct{}) bool {
	if m == nil || len(turnElicitation) == 0 {
		return false
	}
	if isElicitationMessageView(m) {
		return false
	}
	if strings.ToLower(strings.TrimSpace(m.Role)) != "assistant" {
		return false
	}
	if strings.ToLower(strings.TrimSpace(m.Type)) != "text" {
		return false
	}
	if m.TurnId == nil {
		return false
	}
	turnID := strings.TrimSpace(*m.TurnId)
	if turnID == "" {
		return false
	}
	_, ok := turnElicitation[turnID]
	return ok
}

func isElicitationStreamEvent(ev *modelcallctx.StreamEvent) bool {
	if ev == nil || ev.Message == nil {
		return false
	}
	if isElicitationMessageView(ev.Message) {
		return true
	}
	raw := ""
	if ev.Message.Content != nil {
		raw = strings.TrimSpace(*ev.Message.Content)
	}
	if raw == "" && ev.Message.RawContent != nil {
		raw = strings.TrimSpace(*ev.Message.RawContent)
	}
	if looksLikeElicitationStream(raw) {
		return true
	}
	if s, ok := ev.Content.(string); ok {
		if looksLikeElicitationStream(s) {
			return true
		}
	}
	if m, ok := ev.Content.(map[string]interface{}); ok {
		if containsElicitationText(m) {
			return true
		}
	}
	return false
}

func looksLikeElicitationStream(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	if strings.Contains(raw, "```") && strings.Contains(strings.ToLower(raw), "json") {
		if strings.Contains(strings.ToLower(raw), "\"type\"") {
			return true
		}
	}
	if strings.Contains(strings.ToLower(raw), "\"requestedschema\"") {
		return true
	}
	if strings.Contains(raw, "\"type\"") && strings.Contains(strings.ToLower(raw), "elicitation") {
		return true
	}
	return false
}

func eventNameForMessage(m *agconv.MessageView, contentType string) string {
	if m == nil {
		return "message"
	}
	if strings.EqualFold(strings.TrimSpace(contentType), "application/elicitation+json") {
		return "elicitation"
	}
	if m.ToolCall != nil {
		return toolCallEventName(m.ToolCall.Status)
	}
	if m.ModelCall != nil && strings.TrimSpace(ptrString(m.Content)) == "" && strings.TrimSpace(ptrString(m.RawContent)) == "" {
		return modelCallEventName(m.ModelCall.Status)
	}
	if len(m.Attachment) > 0 {
		return "attachment_linked"
	}
	if strings.EqualFold(strings.TrimSpace(m.Role), "assistant") {
		if m.Interim != 0 {
			return "interim_message"
		}
		return "assistant_message"
	}
	if strings.EqualFold(strings.TrimSpace(m.Role), "user") {
		return "user_message"
	}
	return "message"
}

func toolCallEventName(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "ok", "succeeded", "success":
		return "tool_call_completed"
	case "failed", "error":
		return "tool_call_failed"
	case "canceled", "cancelled":
		return "tool_call_failed"
	default:
		return "tool_call_started"
	}
}

func modelCallEventName(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "ok", "succeeded", "success":
		return "model_call_completed"
	case "failed", "error":
		return "model_call_failed"
	case "canceled", "cancelled":
		return "model_call_failed"
	default:
		return "model_call_started"
	}
}

func ptrString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func containsElicitationText(m map[string]interface{}) bool {
	for k, v := range m {
		switch vv := v.(type) {
		case string:
			if looksLikeElicitationStream(vv) {
				return true
			}
		case map[string]interface{}:
			if containsElicitationText(vv) {
				return true
			}
		case []interface{}:
			for _, it := range vv {
				if s, ok := it.(string); ok && looksLikeElicitationStream(s) {
					return true
				}
				if mm, ok := it.(map[string]interface{}); ok {
					if containsElicitationText(mm) {
						return true
					}
				}
			}
		default:
			_ = k
		}
	}
	return false
}

func sanitizeMessageForStream(m *agconv.MessageView) *agconv.MessageView {
	if m == nil {
		return nil
	}
	mc := *m
	if mc.Attachment == nil {
		return &mc
	}
	atts := make([]*agconv.AttachmentView, len(mc.Attachment))
	for i, a := range mc.Attachment {
		if a == nil {
			continue
		}
		ac := *a
		ac.InlineBody = nil
		atts[i] = &ac
	}
	mc.Attachment = atts
	return &mc
}

func (s *Server) nextEventSeq(convID string, msg *agconv.MessageView) uint64 {
	if msg != nil && msg.Sequence != nil && *msg.Sequence > 0 {
		return uint64(*msg.Sequence)
	}
	s.eventSeqMu.Lock()
	defer s.eventSeqMu.Unlock()
	seq := s.eventSeq[convID] + 1
	s.eventSeq[convID] = seq
	return seq
}

func buildStreamContent(m *agconv.MessageView) (string, interface{}) {
	if m == nil {
		return "", nil
	}
	if isElicitationMessageView(m) {
		payload := map[string]interface{}{
			"type": "elicitation",
		}
		if m.ElicitationId != nil && strings.TrimSpace(*m.ElicitationId) != "" {
			payload["elicitationId"] = strings.TrimSpace(*m.ElicitationId)
		}
		if m.Status != nil && strings.TrimSpace(*m.Status) != "" {
			payload["status"] = strings.TrimSpace(*m.Status)
		}
		if m.ElicitationPayloadId != nil && strings.TrimSpace(*m.ElicitationPayloadId) != "" {
			payload["elicitationPayloadId"] = strings.TrimSpace(*m.ElicitationPayloadId)
		}
		if m.UserElicitationData != nil && m.UserElicitationData.InlineBody != nil {
			payload["userPayload"] = *m.UserElicitationData.InlineBody
			if strings.TrimSpace(m.UserElicitationData.Compression) != "" {
				payload["userPayloadCompression"] = strings.TrimSpace(m.UserElicitationData.Compression)
			}
		}
		raw := messageTextForElicitation(m)
		if raw != "" {
			if elicObj, ok := parseElicitationJSON(raw); ok && elicObj != nil {
				payload["elicitation"] = elicObj
			} else {
				payload["message"] = raw
			}
		}
		return "application/elicitation+json", payload
	}
	meta := map[string]interface{}{}
	content := map[string]interface{}{}
	switch strings.ToLower(strings.TrimSpace(m.Type)) {
	case "text":
		if isPreambleMessage(m) {
			meta["kind"] = "preamble"
		}
		txt := ""
		if m.Content != nil {
			txt = strings.TrimSpace(*m.Content)
		}
		if txt == "" && m.RawContent != nil {
			txt = strings.TrimSpace(*m.RawContent)
		}
		if txt != "" {
			content["text"] = txt
		}
		if len(meta) > 0 {
			content["meta"] = meta
		}
	case "tool_op":
		if m.ToolCall != nil {
			if phase := toolPhaseFromMessage(m); phase != "" {
				meta["phase"] = phase
			}
			content["toolCallId"] = strings.TrimSpace(m.ToolCall.OpId)
			content["name"] = strings.TrimSpace(m.ToolCall.ToolName)
			if m.ToolCall.RequestPayload != nil && m.ToolCall.RequestPayload.InlineBody != nil {
				content["request"] = *m.ToolCall.RequestPayload.InlineBody
				content["requestCompression"] = strings.TrimSpace(m.ToolCall.RequestPayload.Compression)
			}
			if m.ToolCall.ResponsePayload != nil && m.ToolCall.ResponsePayload.InlineBody != nil {
				content["response"] = *m.ToolCall.ResponsePayload.InlineBody
				content["responseCompression"] = strings.TrimSpace(m.ToolCall.ResponsePayload.Compression)
			}
			if m.ToolCall.ErrorMessage != nil && strings.TrimSpace(*m.ToolCall.ErrorMessage) != "" {
				content["error"] = strings.TrimSpace(*m.ToolCall.ErrorMessage)
			}
			if len(meta) > 0 {
				content["meta"] = meta
			}
		}
	}
	if len(content) == 0 {
		return "", nil
	}
	return "application/json", content
}

func isElicitationMessageView(m *agconv.MessageView) bool {
	if m == nil {
		return false
	}
	if m.ElicitationId != nil && strings.TrimSpace(*m.ElicitationId) != "" {
		return true
	}
	t := agconv.MessageType(m.Type)
	return t.IsElicitationRequest() || t.IsElicitationResponse()
}

func messageTextForElicitation(m *agconv.MessageView) string {
	if m == nil {
		return ""
	}
	if m.Content != nil && strings.TrimSpace(*m.Content) != "" {
		return strings.TrimSpace(*m.Content)
	}
	if m.RawContent != nil && strings.TrimSpace(*m.RawContent) != "" {
		return strings.TrimSpace(*m.RawContent)
	}
	return ""
}

func parseElicitationJSON(raw string) (map[string]interface{}, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false
	}
	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```")
		raw = strings.TrimSpace(raw)
		if strings.HasPrefix(raw, "json") {
			raw = strings.TrimSpace(strings.TrimPrefix(raw, "json"))
		}
		if idx := strings.LastIndex(raw, "```"); idx >= 0 {
			raw = strings.TrimSpace(raw[:idx])
		}
	}
	if !strings.HasPrefix(raw, "{") {
		return nil, false
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, false
	}
	typ, _ := payload["type"].(string)
	if strings.EqualFold(strings.TrimSpace(typ), "elicitation") {
		return payload, true
	}
	return nil, false
}

func isPreambleMessage(m *agconv.MessageView) bool {
	if m == nil {
		return false
	}
	if strings.ToLower(strings.TrimSpace(m.Role)) != "assistant" {
		return false
	}
	if strings.ToLower(strings.TrimSpace(m.Type)) != "text" {
		return false
	}
	if m.Interim == 0 {
		return false
	}
	if m.ModelCall == nil || len(m.ModelCall.ToolCallLinks) == 0 {
		return false
	}
	return true
}

func toolPhaseFromMessage(m *agconv.MessageView) string {
	if m == nil || m.ToolCall == nil {
		return ""
	}
	status := strings.ToLower(strings.TrimSpace(m.ToolCall.Status))
	switch status {
	case "running", "pending":
		return "request"
	case "completed", "succeeded", "success", "failed", "error", "canceled", "cancelled":
		return "response"
	}
	if m.ToolCall.ResponsePayload != nil && m.ToolCall.ResponsePayload.InlineBody != nil {
		return "response"
	}
	if m.ToolCall.RequestPayload != nil && m.ToolCall.RequestPayload.InlineBody != nil {
		return "request"
	}
	return ""
}

func (s *Server) findLastMessageID(ctx context.Context, convID string, includeModelCallPayload bool) string {
	if s.chatSvc == nil {
		return ""
	}
	conv, err := s.chatSvc.Get(ctx, chat.GetRequest{
		ConversationID:          convID,
		IncludeModelCallPayload: includeModelCallPayload,
		IncludeToolCall:         false,
	})
	if err != nil || conv == nil || conv.Conversation == nil {
		return ""
	}
	msgs := flattenTranscriptMessages(conv.Conversation)
	if len(msgs) == 0 {
		return ""
	}
	last := msgs[len(msgs)-1]
	if last == nil {
		return ""
	}
	return last.Id
}

func (s *Server) findLastMessageSeq(ctx context.Context, convID string, includeModelCallPayload bool) uint64 {
	if s.chatSvc == nil {
		return 0
	}
	conv, err := s.chatSvc.Get(ctx, chat.GetRequest{
		ConversationID:          convID,
		IncludeModelCallPayload: includeModelCallPayload,
		IncludeToolCall:         false,
	})
	if err != nil || conv == nil || conv.Conversation == nil {
		return 0
	}
	msgs := flattenTranscriptMessages(conv.Conversation)
	if len(msgs) == 0 {
		return 0
	}
	last := msgs[len(msgs)-1]
	if last == nil || last.Sequence == nil || *last.Sequence <= 0 {
		return 0
	}
	return uint64(*last.Sequence)
}

func (s *Server) collectEventEnvelopes(ctx context.Context, convID string, includeTypes map[string]struct{}, includeHistory, includeModelCallPayload bool, lastSeenSeq uint64, hasSeq bool, lastSeenID string) ([]*streamMessageEnvelope, uint64, string, error) {
	conv, err := s.chatSvc.Get(ctx, chat.GetRequest{
		ConversationID:          convID,
		SinceID:                 lastSeenID,
		IncludeModelCallPayload: includeModelCallPayload,
		IncludeToolCall:         true,
	})
	if err != nil {
		return nil, lastSeenSeq, lastSeenID, err
	}
	if conv == nil || conv.Conversation == nil {
		return nil, lastSeenSeq, lastSeenID, fmt.Errorf("conversation not found")
	}
	messages := flattenTranscriptMessages(conv.Conversation)
	start := 0
	if !hasSeq && lastSeenID != "" {
		for i, m := range messages {
			if m == nil {
				continue
			}
			if m.Id == lastSeenID {
				start = i + 1
				break
			}
		}
	}
	var out []*streamMessageEnvelope
	for i := start; i < len(messages); i++ {
		m := messages[i]
		if m == nil || strings.TrimSpace(m.Id) == "" {
			continue
		}
		seq := s.nextEventSeq(convID, m)
		if hasSeq && seq <= lastSeenSeq {
			continue
		}
		if len(includeTypes) > 0 {
			msgType := strings.ToLower(strings.TrimSpace(m.Type))
			if _, ok := includeTypes[msgType]; !ok {
				if _, ok := includeTypes["elicitation"]; !ok || !isElicitationMessageView(m) {
					continue
				}
			}
		}
		out = append(out, &streamMessageEnvelope{
			Seq:            seq,
			Time:           time.Now().UTC(),
			ConversationID: convID,
			Message:        sanitizeMessageForStream(m),
		})
		if ctype, content := buildStreamContent(m); content != nil {
			out[len(out)-1].ContentType = ctype
			out[len(out)-1].Content = content
		}
		lastSeenSeq = seq
		hasSeq = true
		lastSeenID = m.Id
	}
	if len(out) == 0 && !includeHistory && !hasSeq && lastSeenID == "" {
		if seq := s.findLastMessageSeq(ctx, convID, includeModelCallPayload); seq > 0 {
			lastSeenSeq = seq
			hasSeq = true
		} else if id := s.findLastMessageID(ctx, convID, includeModelCallPayload); id != "" {
			lastSeenID = id
		}
	}
	return out, lastSeenSeq, lastSeenID, nil
}

// sanitizeConversationForAPI returns a copy of conv with any binary attachment
// bytes removed, so transcripts can be returned over HTTP without flooding
// clients with base64 payloads.
func sanitizeConversationForAPI(conv *apiconv.Conversation) *apiconv.Conversation {
	if conv == nil || conv.Transcript == nil {
		return conv
	}
	out := *conv
	tr := make([]*agconv.TranscriptView, len(conv.Transcript))
	for i, t := range conv.Transcript {
		if t == nil {
			continue
		}
		tc := *t
		if t.Message != nil {
			msgs := make([]*agconv.MessageView, 0, len(t.Message))
			for _, m := range t.Message {
				if m == nil {
					continue
				}
				if isSummaryMessageView(m) {
					continue
				}
				mc := *m
				if mc.Attachment != nil {
					atts := make([]*agconv.AttachmentView, len(mc.Attachment))
					for k, a := range mc.Attachment {
						if a == nil {
							continue
						}
						ac := *a
						ac.InlineBody = nil
						atts[k] = &ac
					}
					mc.Attachment = atts
				}
				msgs = append(msgs, &mc)
			}
			tc.Message = msgs
		}
		tr[i] = &tc
	}
	out.Transcript = tr
	return &out
}

func isSummaryMessageView(m *agconv.MessageView) bool {
	if m == nil {
		return false
	}
	if m.Mode != nil && strings.EqualFold(strings.TrimSpace(*m.Mode), "summary") {
		return true
	}
	if m.Status != nil && strings.EqualFold(strings.TrimSpace(*m.Status), "summary") {
		return true
	}
	return false
}

func isInterimAssistantMessageView(m *agconv.MessageView) bool {
	if m == nil {
		return false
	}
	if m.Interim == 0 {
		return false
	}
	if strings.ToLower(strings.TrimSpace(m.Role)) != "assistant" {
		return false
	}
	if strings.ToLower(strings.TrimSpace(m.Type)) != "text" {
		return false
	}
	return true
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

// handleDeleteTurn cancels a queued turn by id within a conversation.
func (s *Server) handleDeleteTurn(w http.ResponseWriter, r *http.Request, convID, turnID string) {
	if r.Method != http.MethodDelete {
		encode(w, http.StatusMethodNotAllowed, nil, fmt.Errorf("method not allowed"))
		return
	}
	if strings.TrimSpace(convID) == "" || strings.TrimSpace(turnID) == "" {
		encode(w, http.StatusBadRequest, nil, fmt.Errorf("conversation and turn id required"))
		return
	}
	if s.chatSvc == nil {
		encode(w, http.StatusInternalServerError, nil, fmt.Errorf("chat service not initialised"))
		return
	}
	if err := s.chatSvc.CancelQueuedTurn(r.Context(), convID, turnID); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			encode(w, http.StatusNotFound, nil, err)
			return
		}
		if strings.Contains(strings.ToLower(err.Error()), "cannot cancel") {
			encode(w, http.StatusBadRequest, nil, err)
			return
		}
		encode(w, http.StatusInternalServerError, nil, err)
		return
	}
	encode(w, http.StatusOK, map[string]string{"id": turnID, "status": "canceled"}, nil)
}

type moveQueuedTurnRequest struct {
	Direction string `json:"direction"`
}

func (s *Server) handleMoveQueuedTurn(w http.ResponseWriter, r *http.Request, convID, turnID string) {
	if r.Method != http.MethodPost {
		encode(w, http.StatusMethodNotAllowed, nil, fmt.Errorf("method not allowed"))
		return
	}
	if strings.TrimSpace(convID) == "" || strings.TrimSpace(turnID) == "" {
		encode(w, http.StatusBadRequest, nil, fmt.Errorf("conversation and turn id required"))
		return
	}
	if s.chatSvc == nil {
		encode(w, http.StatusInternalServerError, nil, fmt.Errorf("chat service not initialised"))
		return
	}
	var req moveQueuedTurnRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		encode(w, http.StatusBadRequest, nil, fmt.Errorf("invalid request body: %w", err))
		return
	}
	if err := s.chatSvc.MoveQueuedTurn(r.Context(), convID, turnID, req.Direction); err != nil {
		encode(w, http.StatusBadRequest, nil, err)
		return
	}
	encode(w, http.StatusOK, map[string]string{"id": turnID, "status": "moved"}, nil)
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
		Reason  string                 `json:"reason,omitempty"`
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
	if err := eliciationService.Resolve(r.Context(), convID, elicID, req.Action, req.Payload, req.Reason); err != nil {
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
					if token, err := store.EnsureToken(ctx, uid, prov, s.authCfg.OAuth.Client.ConfigURL); err == nil && token != nil {
						ctx = auth.WithBearer(ctx, token.AccessToken)
						if token.IDToken != "" {
							ctx = auth.WithIDToken(ctx, token.IDToken)
						}
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
	// Preserve request context (auth/user) when patching the last turn/message; fall back to background only
	// if the request context is already canceled.
	ctx := r.Context()
	if ctx.Err() != nil {
		ctx = context.Background()
	}
	if err := s.chatSvc.SetLastAssistentMessageStatus(ctx, convID, "canceled"); err != nil {
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

// handlePruneConversation processes POST /v1/api/conversations/{id}/prune
// and triggers server-side pruning: removes low-value messages via LLM selection.
// Returns 202 on success.
func (s *Server) handlePruneConversation(w http.ResponseWriter, r *http.Request, convID string) {
	if s == nil || s.chatSvc == nil || strings.TrimSpace(convID) == "" {
		encode(w, http.StatusBadRequest, nil, fmt.Errorf("invalid conversation id"))
		return
	}

	s.pruneGuardMu.Lock()
	g := s.pruneGuards[convID]
	if g == nil {
		var v int32
		g = &v
		s.pruneGuards[convID] = g
	}
	s.pruneGuardMu.Unlock()

	if !atomic.CompareAndSwapInt32(g, 0, 1) {
		// Another prune in progress; treat as success (idempotent)
		encode(w, http.StatusAccepted, map[string]any{"pruned": true}, nil)
		return
	}
	defer atomic.StoreInt32(g, 0)

	ctx := r.Context()
	if err := s.chatSvc.Prune(ctx, convID); err != nil {
		encode(w, http.StatusInternalServerError, nil, err)
		return
	}
	encode(w, http.StatusAccepted, map[string]any{"pruned": true}, nil)
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
