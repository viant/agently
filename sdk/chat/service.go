package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"sync"

	"github.com/google/uuid"
	mcpclient "github.com/viant/agently/adapter/mcp"
	plan "github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/conversation"
	"github.com/viant/agently/genai/memory"
	agentpkg "github.com/viant/agently/genai/service/agent"
	"github.com/viant/agently/genai/tool"
	authctx "github.com/viant/agently/internal/auth"
	convread "github.com/viant/agently/internal/dao/conversation/read"
	convw "github.com/viant/agently/internal/dao/conversation/write"
	msgread "github.com/viant/agently/internal/dao/message/read"
	msgwrite "github.com/viant/agently/internal/dao/message/write"
	plread "github.com/viant/agently/internal/dao/payload/read"
	usageread "github.com/viant/agently/internal/dao/usage/read"
	d "github.com/viant/agently/internal/domain"
	"github.com/viant/agently/sdk/stage"
	fluxpol "github.com/viant/fluxor/policy"
	"github.com/viant/fluxor/service/approval"
	"github.com/viant/mcp-protocol/schema"
)

// Service exposes message retrieval independent of HTTP concerns.
type Service struct {
	store      d.Store
	mgr        *conversation.Manager
	toolPolicy *tool.Policy
	fluxPolicy *fluxpol.Policy
	approval   approval.Service

	mu            sync.Mutex
	cancelsByTurn map[string][]context.CancelFunc // key: user turn id (message id)
	turnsByConv   map[string][]string             // convID -> []turnID
}

func NewService(store d.Store) *Service { return &Service{store: store} }

// AttachManager configures the conversation manager and optional default policies.
func (s *Service) AttachManager(mgr *conversation.Manager, tp *tool.Policy, fp *fluxpol.Policy) {
	s.mgr = mgr
	s.toolPolicy = tp
	s.fluxPolicy = fp
}

// AttachApproval configures the approval service bridge for policy decisions.
func (s *Service) AttachApproval(svc approval.Service) { s.approval = svc }

// GetRequest defines inputs to fetch messages.
type GetRequest struct {
	ConversationID string
	SinceID        string // optional: inclusive slice starting from this message id
}

// GetResponse carries fetched messages, inferred stage and progress flag.
type GetResponse struct {
	Messages   []memory.Message
	Stage      *stage.Stage
	InProgress bool // true when SinceID set and no new messages yet
}

// Get fetches messages according to request and computes conversation stage.
func (s *Service) Get(ctx context.Context, req GetRequest) (*GetResponse, error) {
	opts := []msgread.InputOption{msgread.WithConversationID(req.ConversationID)}
	if id := strings.TrimSpace(req.SinceID); id != "" {
		// Normalize synthetic suffixes like "/form" or "/1".
		if idx := strings.IndexByte(id, '/'); idx > 0 {
			id = id[:idx]
		}
		// If since points to a non-user row (e.g., assistant or child), anchor to its parent user message.
		if rows, err := s.store.Messages().List(ctx, msgread.WithIDs(id)); err == nil && len(rows) > 0 && rows[0] != nil {
			v := rows[0]
			if !strings.EqualFold(strings.TrimSpace(v.Role), "user") {
				if v.ParentID != nil && strings.TrimSpace(*v.ParentID) != "" {
					id = *v.ParentID
				}
			}
		}
		opts = append(opts, msgread.WithSinceID(id))
	}

	transcript, err := s.store.Messages().GetTranscript(ctx, req.ConversationID, opts...)
	if err != nil {
		return nil, err
	}

	var execution = make(map[string][]*msgread.MessageView)
	var turns []*msgread.MessageView
	for i, v := range transcript {
		// Treat interim children and tool-call rows as execution details under their parent.
		if (v.IsInterim() && v.ParentID != nil) || v.ToolCall != nil {
			execution[*v.ParentID] = append(execution[*v.ParentID], transcript[i])
			continue
		}
		// Some backends emit synthetic child message IDs (e.g., "<parent>/1") without setting ParentID.
		// Infer parent from ID when ParentID is missing to avoid duplicate user-visible bubbles.
		if v.ParentID == nil {
			if slash := strings.IndexByte(v.Id, '/'); slash > 0 {
				parent := v.Id[:slash]
				execution[parent] = append(execution[parent], transcript[i])
				continue
			}
		}
		turns = append(turns, transcript[i])
	}
	messages := make([]memory.Message, 0, len(transcript))

	for _, v := range turns {
		if v == nil {
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

		if executions, ok := execution[v.Id]; ok {
			for _, anExecution := range executions {
				if anExecution.ToolCall != nil {
					st := &plan.StepOutcome{
						ID:                anExecution.ToolCall.OpID,
						Name:              anExecution.ToolCall.ToolName,
						Reason:            "tool_call",
						Success:           strings.EqualFold(strings.TrimSpace(anExecution.ToolCall.Status), "completed"),
						Error:             derefStr(anExecution.ToolCall.ErrorMessage),
						StartedAt:         anExecution.ToolCall.StartedAt,
						EndedAt:           anExecution.ToolCall.CompletedAt,
						RequestPayloadID:  anExecution.ToolCall.RequestPayloadID,
						ResponsePayloadID: anExecution.ToolCall.ResponsePayloadID,
					}
					if anExecution.ToolCall.StartedAt != nil && anExecution.ToolCall.CompletedAt != nil {
						st.Elapsed = anExecution.ToolCall.CompletedAt.Sub(*anExecution.ToolCall.StartedAt).Round(time.Millisecond).String()
					}
					mm.Executions = append(mm.Executions, &plan.Outcome{Steps: []*plan.StepOutcome{st}})
				}
				if anExecution.ModelCall != nil {
					st := &plan.StepOutcome{
						ID:      anExecution.ModelCall.MessageID,
						Name:    anExecution.ModelCall.Model,
						Reason:  "thinking",
						Success: strings.EqualFold(strings.TrimSpace(anExecution.ModelCall.Status), "completed"),
						//TODO add to model_calls
						//	Error:             derefStr(v.ModelCall.ErrorMessage),
						StartedAt:         anExecution.ModelCall.StartedAt,
						EndedAt:           anExecution.ModelCall.CompletedAt,
						RequestPayloadID:  anExecution.ModelCall.RequestPayloadID,
						ResponsePayloadID: anExecution.ModelCall.ResponsePayloadID,
						StreamPayloadID:   anExecution.ModelCall.StreamPayloadID,
					}
					if anExecution.ModelCall.StartedAt != nil && anExecution.ModelCall.CompletedAt != nil {
						st.Elapsed = anExecution.ModelCall.CompletedAt.Sub(*anExecution.ModelCall.StartedAt).Round(time.Millisecond).String()
					}
					mm.Executions = append(mm.Executions, &plan.Outcome{Steps: []*plan.StepOutcome{st}})

					// Attach per-message usage when tokens are present
					if anExecution.ModelCall.PromptTokens != nil || anExecution.ModelCall.CompletionTokens != nil || anExecution.ModelCall.TotalTokens != nil {
						var pt, ct, tt int
						if anExecution.ModelCall.PromptTokens != nil {
							pt = *anExecution.ModelCall.PromptTokens
						}
						if anExecution.ModelCall.CompletionTokens != nil {
							ct = *anExecution.ModelCall.CompletionTokens
						}
						if anExecution.ModelCall.TotalTokens != nil {
							tt = *anExecution.ModelCall.TotalTokens
						} else {
							tt = pt + ct
						}
						mm.Usage = &memory.Usage{PromptTokens: pt, CompletionTokens: ct, TotalTokens: tt}
					}
				}
			}
		}
		messages = append(messages, mm)
	}

	stg := s.currentStage(ctx, req.ConversationID)
	inProgress := strings.TrimSpace(req.SinceID) != "" && len(messages) == 0
	return &GetResponse{Messages: messages, Stage: stg, InProgress: inProgress}, nil
}

func (s *Service) inlinePayload(ctx context.Context, id *string, out *json.RawMessage) error {
	if id == nil || strings.TrimSpace(*id) == "" {
		return nil
	}
	pv, err := s.store.Payloads().Get(ctx, *id)
	if err != nil {
		return err
	}
	if pv != nil && pv.InlineBody != nil {
		*out = json.RawMessage(*pv.InlineBody)
	}
	return nil
}

// currentStage infers the live phase of a conversation using transcript signals.
func (s *Service) currentStage(ctx context.Context, convID string) *stage.Stage {
	st := &stage.Stage{Phase: stage.StageWaiting}
	if s == nil || s.store == nil || strings.TrimSpace(convID) == "" {
		return st
	}
	views, err := s.store.Messages().GetTranscript(ctx, convID)
	if err != nil || len(views) == 0 {
		return st
	}
	lastRole := ""
	lastAssistantElic := false
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
		if v.ToolCall != nil {
			status := strings.ToLower(strings.TrimSpace(v.ToolCall.Status))
			if status == "running" || v.ToolCall.CompletedAt == nil {
				lastToolRunning = true
				break
			}
			if status == "failed" {
				lastToolFailed = true
			}
		}
		if v.ModelCall != nil {
			mstatus := strings.ToLower(strings.TrimSpace(v.ModelCall.Status))
			if mstatus == "running" || v.ModelCall.CompletedAt == nil {
				lastModelRunning = true
				break
			}
		}
		if r == "assistant" && v.Elicitation != nil {
			lastAssistantElic = true
			break
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
	return st
}

func derefStr(p *string) string {
	if p != nil {
		return *p
	}
	return ""
}

// PostRequest defines inputs to submit a user message.
type PostRequest struct {
	Content string                 `json:"content"`
	Agent   string                 `json:"agent,omitempty"`
	Model   string                 `json:"model,omitempty"`
	Tools   []string               `json:"tools,omitempty"`
	Context map[string]interface{} `json:"context,omitempty"`
}

// defaultLocation returns supplied if not empty (preserving explicit agent location).
func defaultLocation(loc string) string { return strings.TrimSpace(loc) }

// Post accepts a user message and triggers asynchronous processing via manager.
// Returns generated message ID that can be used to track status.
func (s *Service) Post(ctx context.Context, conversationID string, req PostRequest) (string, error) {
	if s == nil || s.mgr == nil {
		return "", nil
	}
	msgID := uuid.New().String()
	input := &agentpkg.QueryInput{
		ConversationID: conversationID,
		Query:          req.Content,
		AgentName:      defaultLocation(req.Agent),
		ModelOverride:  req.Model,
		ToolsAllowed:   req.Tools,
		Context:        req.Context,
		MessageID:      msgID,
	}

	// Start from background to avoid cancellation when HTTP context ends,
	// then copy relevant values (auth, policies) from parent.
	runCtx, cancel := context.WithCancel(ctx)
	s.registerCancel(conversationID, msgID, cancel)
	defer func() {
		s.completeCancel(conversationID, msgID, cancel)
	}()
	// Propagate conversation ID and policies
	runCtx = conversation.WithID(runCtx, conversationID)
	// Defaults
	if s.toolPolicy != nil {
		runCtx = tool.WithPolicy(runCtx, s.toolPolicy)
	} else {
		runCtx = tool.WithPolicy(runCtx, &tool.Policy{Mode: tool.ModeAuto})
	}
	if pol := tool.FromContext(ctx); pol != nil {
		runCtx = tool.WithPolicy(runCtx, pol)
	}
	if s.fluxPolicy != nil {
		runCtx = fluxpol.WithPolicy(runCtx, s.fluxPolicy)
	}
	_, _ = s.mgr.Accept(runCtx, input)

	return msgID, nil
}

// Cancel aborts all in-flight turns for the given conversation; returns true if any were cancelled.
func (s *Service) Cancel(conversationID string) bool {
	if s == nil {
		return false
	}
	cancels := s.popCancelsByConversation(conversationID)
	for _, c := range cancels {
		if c != nil {
			c()
		}
	}
	return len(cancels) > 0
}

func (s *Service) registerCancel(convID, turnID string, cancel context.CancelFunc) {
	if cancel == nil || strings.TrimSpace(turnID) == "" {
		return
	}
	s.mu.Lock()
	if s.cancelsByTurn == nil {
		s.cancelsByTurn = map[string][]context.CancelFunc{}
	}
	s.cancelsByTurn[turnID] = append(s.cancelsByTurn[turnID], cancel)
	if strings.TrimSpace(convID) != "" {
		if s.turnsByConv == nil {
			s.turnsByConv = map[string][]string{}
		}
		s.turnsByConv[convID] = append(s.turnsByConv[convID], turnID)
	}
	s.mu.Unlock()
}

func (s *Service) completeCancel(convID, turnID string, cancel context.CancelFunc) {
	s.mu.Lock()
	if s.cancelsByTurn != nil {
		list := s.cancelsByTurn[turnID]
		for i, c := range list {
			if fmt.Sprintf("%p", c) == fmt.Sprintf("%p", cancel) {
				list = append(list[:i], list[i+1:]...)
				break
			}
		}
		if len(list) == 0 {
			delete(s.cancelsByTurn, turnID)
		} else {
			s.cancelsByTurn[turnID] = list
		}
	}
	s.mu.Unlock()
}

// CancelTurn aborts a specific user turn (keyed by messageId) if running.
func (s *Service) CancelTurn(turnID string) bool {
	s.mu.Lock()
	var list []context.CancelFunc
	if s.cancelsByTurn != nil {
		list = s.cancelsByTurn[turnID]
		delete(s.cancelsByTurn, turnID)
	}
	s.mu.Unlock()
	for _, c := range list {
		if c != nil {
			c()
		}
	}
	return len(list) > 0
}

func (s *Service) popCancelsByConversation(convID string) []context.CancelFunc {
	s.mu.Lock()
	var result []context.CancelFunc
	if s.turnsByConv != nil && s.cancelsByTurn != nil {
		turns := s.turnsByConv[convID]
		delete(s.turnsByConv, convID)
		for _, tID := range turns {
			if list, ok := s.cancelsByTurn[tID]; ok {
				result = append(result, list...)
				delete(s.cancelsByTurn, tID)
			}
		}
	}
	s.mu.Unlock()
	return result
}

// --------------------------
// Conversations API
// --------------------------

// CreateConversationRequest mirrors HTTP payload for POST /conversations.
type CreateConversationRequest struct {
	Model      string `json:"model"`
	Agent      string `json:"agent"`
	Tools      string `json:"tools"` // comma-separated
	Title      string `json:"title"`
	Visibility string `json:"visibility"`
}

// CreateConversationResponse echoes created entity details.
type CreateConversationResponse struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CreatedAt string `json:"createdAt"`
	Model     string `json:"model,omitempty"`
	Agent     string `json:"agent,omitempty"`
	Tools     string `json:"tools,omitempty"`
}

// ConversationSummary lists id + title only.
type ConversationSummary struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// CreateConversation persists a new conversation using DAO store.
func (s *Service) CreateConversation(ctx context.Context, in CreateConversationRequest) (*CreateConversationResponse, error) {
	if s.store == nil {
		return nil, fmt.Errorf("store not initialised")
	}
	id := uuid.NewString()
	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = fmt.Sprintf("Conversation at %s", humanTimestamp(time.Now()))
	}
	createdAt := time.Now().UTC()
	cw := &convw.Conversation{Has: &convw.ConversationHas{}}
	cw.SetId(id)
	cw.SetTitle(title)
	cw.SetCreatedAt(createdAt)
	// Persist created_by_user_id when present in context
	if ui := authctx.User(ctx); ui != nil {
		userID := strings.TrimSpace(ui.Subject)
		if userID == "" {
			userID = strings.TrimSpace(ui.Email)
		}
		if userID != "" {
			cw.SetCreatedByUserID(userID)
		}
	}
	if strings.TrimSpace(in.Visibility) == "" {
		cw.SetVisibility(convw.VisibilityPublic)
	} else {
		cw.SetVisibility(strings.TrimSpace(in.Visibility))
	}
	if s := strings.TrimSpace(in.Agent); s != "" {
		cw.SetAgentName(s)
	}
	if s := strings.TrimSpace(in.Model); s != "" {
		cw.SetDefaultModel(s)
	}
	if s := strings.TrimSpace(in.Tools); s != "" {
		parts := strings.Split(s, ",")
		tools := make([]string, 0, len(parts))
		for _, p := range parts {
			if v := strings.TrimSpace(p); v != "" {
				tools = append(tools, v)
			}
		}
		if len(tools) > 0 {
			meta := map[string]any{"tools": tools}
			if b, err := json.Marshal(meta); err == nil {
				cw.SetMetadata(string(b))
			}
		}
	}
	if _, err := s.store.Conversations().Patch(ctx, cw); err != nil {
		return nil, fmt.Errorf("failed to persist conversation: %w", err)
	}
	return &CreateConversationResponse{ID: id, Title: title, CreatedAt: createdAt.Format(time.RFC3339), Model: in.Model, Agent: in.Agent, Tools: in.Tools}, nil
}

// GetConversation returns id + title by conversation id.
func (s *Service) GetConversation(ctx context.Context, id string) (*ConversationSummary, error) {
	if s.store == nil {
		return nil, fmt.Errorf("store not initialised")
	}
	cv, err := s.store.Conversations().Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if cv == nil {
		return nil, nil
	}
	t := id
	if cv.Title != nil && strings.TrimSpace(*cv.Title) != "" {
		t = *cv.Title
	}
	return &ConversationSummary{ID: id, Title: t}, nil
}

// ListConversations returns all conversation summaries.
func (s *Service) ListConversations(ctx context.Context) ([]ConversationSummary, error) {
	if s.store == nil {
		return nil, fmt.Errorf("store not initialised")
	}
	opts := []convread.ConversationInputOption{convread.WithArchived(0, 1)}
	// Authorize list: show user's own OR public
	if ui := authctx.User(ctx); ui != nil {
		uid := strings.TrimSpace(ui.Subject)
		if uid == "" {
			uid = strings.TrimSpace(ui.Email)
		}
		if uid != "" {
			opts = append(opts, convread.WithCreatedByUserID(uid))
			opts = append(opts, convread.WithVisibility(convw.VisibilityPublic))
		}
	} else {
		// No user context: default to public only
		opts = append(opts, convread.WithVisibility(convw.VisibilityPublic))
	}
	rows, err := s.store.Conversations().List(ctx, opts...)
	if err != nil {
		return nil, err
	}
	out := make([]ConversationSummary, 0, len(rows))
	for _, v := range rows {
		if v == nil {
			continue
		}
		t := v.Id
		if v.Title != nil && strings.TrimSpace(*v.Title) != "" {
			t = *v.Title
		}
		out = append(out, ConversationSummary{ID: v.Id, Title: t})
	}
	return out, nil
}

// humanTimestamp formats a friendly timestamp used for default conversation titles.
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
	return fmt.Sprintf("%s %s %d%s %d, %02d:%02d", t.Weekday().String()[:3], t.Month().String(), day, suffix, t.Year(), t.Hour(), t.Minute())
}

// Approve processes an approval decision for a message. It acknowledges
// "cancel" without persisting any changes; for accept/decline it stores the
// status and forwards to the approval service when configured.
func (s *Service) Approve(ctx context.Context, messageID, action, reason string) error {
	action = strings.ToLower(strings.TrimSpace(action))
	switch action {
	case "cancel":
		// Acknowledge without persisting or forwarding.
		return nil
	case "accept", "approve", "approved", "yes", "y", "decline", "deny", "reject", "no", "n":
		// proceed
	default:
		return fmt.Errorf("invalid action")
	}

	// Map to status and approved flag
	approved := action == "accept" || action == "approve" || action == "approved" || action == "yes" || action == "y"
	newStatus := "declined"
	if approved {
		newStatus = "done"
	}

	// Verify message exists and patch status
	rows, err := s.store.Messages().List(ctx, msgread.WithIDs(messageID))
	if err != nil {
		return err
	}
	if len(rows) == 0 || rows[0] == nil {
		return fmt.Errorf("interaction message not found")
	}
	msg := rows[0]
	_, _ = s.store.Messages().Patch(ctx, &msgwrite.Message{Id: msg.Id, Status: newStatus, Has: &msgwrite.MessageHas{Status: true}})

	if s.approval != nil {
		_, _ = s.approval.Decide(ctx, messageID, approved, reason)
	}
	return nil
}

// Elicit processes an elicitation decision (accept/decline/cancel) and forwards
// the result to an MCP waiter if present.
func (s *Service) Elicit(ctx context.Context, messageID, action string, payload map[string]interface{}) error {
	action = strings.ToLower(strings.TrimSpace(action))
	if action == "" {
		return fmt.Errorf("action is required")
	}
	rows, err := s.store.Messages().List(ctx, msgread.WithIDs(messageID))
	if err != nil {
		return err
	}
	if len(rows) == 0 || rows[0] == nil {
		return fmt.Errorf("interaction message not found")
	}
	msg := rows[0]
	status := "declined"
	if action == "accept" {
		status = "done"
	}
	_, _ = s.store.Messages().Patch(ctx, &msgwrite.Message{Id: msg.Id, Status: status, Has: &msgwrite.MessageHas{Status: true}})
	if ch, ok := mcpclient.Waiter(messageID); ok {
		ch <- &schema.ElicitResult{Action: schema.ElicitResultAction(action), Content: payload}
	}
	return nil
}

// GetMessage fetches a single message by id and, when present, decodes
// elicitation payload into a typed structure.
func (s *Service) GetMessage(ctx context.Context, id string) (*memory.Message, error) {
	rows, err := s.store.Messages().List(ctx, msgread.WithIDs(id))
	if err != nil || len(rows) == 0 || rows[0] == nil {
		return nil, err
	}
	v := rows[0]
	mm := memory.Message{ID: v.Id, ConversationID: v.ConversationID, Role: v.Role, Content: v.Content}
	if v.CreatedAt != nil {
		mm.CreatedAt = *v.CreatedAt
	}
	if v.ToolName != nil {
		mm.ToolName = v.ToolName
	}
	// Inline elicitation when payload present
	if v.ElicitationID != nil {
		if pv, e := s.store.Payloads().List(ctx, plread.WithID(*v.ElicitationID)); e == nil && len(pv) > 0 && pv[0] != nil && pv[0].InlineBody != nil {
			var ecli plan.Elicitation
			if json.Unmarshal(*pv[0].InlineBody, &ecli) == nil {
				mm.Elicitation = &ecli
			}
		} else if e != nil {
			return nil, e
		}
	}
	return &mm, nil
}

// UsagePerModel represents token statistics for a single model.
type UsagePerModel struct {
	Model           string `json:"model"`
	InputTokens     int    `json:"inputTokens"`
	OutputTokens    int    `json:"outputTokens"`
	EmbeddingTokens int    `json:"embeddingTokens"`
	CachedTokens    int    `json:"cachedTokens"`
}

// Usage aggregates conversation usage totals and per-model breakdown.
type Usage struct {
	ConversationID  string          `json:"conversationId"`
	InputTokens     int             `json:"inputTokens"`
	OutputTokens    int             `json:"outputTokens"`
	EmbeddingTokens int             `json:"embeddingTokens"`
	CachedTokens    int             `json:"cachedTokens"`
	TotalTokens     int             `json:"totalTokens"`
	PerModel        []UsagePerModel `json:"perModel"`
}

// GetUsage aggregates token usage per model and totals for a conversation.
func (s *Service) GetUsage(ctx context.Context, conversationID string) (*Usage, error) {
	in := usageread.Input{ConversationID: conversationID, Has: &usageread.Has{ConversationID: true}}
	rows, err := s.store.Usage().List(ctx, in)
	if err != nil {
		return nil, err
	}
	out := &Usage{ConversationID: conversationID, PerModel: []UsagePerModel{}}
	for _, v := range rows {
		if v == nil {
			continue
		}
		pm := UsagePerModel{Model: strings.TrimSpace(v.Provider + "/" + v.Model)}
		if v.TotalPromptTokens != nil {
			pm.InputTokens = *v.TotalPromptTokens
			out.InputTokens += *v.TotalPromptTokens
		}
		if v.TotalCompletionTokens != nil {
			pm.OutputTokens = *v.TotalCompletionTokens
			out.OutputTokens += *v.TotalCompletionTokens
		}
		if (pm.InputTokens+pm.OutputTokens) == 0 && v.TotalTokens != nil {
			pm.OutputTokens += *v.TotalTokens
			out.OutputTokens += *v.TotalTokens
		}
		out.PerModel = append(out.PerModel, pm)
	}
	out.TotalTokens = out.InputTokens + out.OutputTokens + out.EmbeddingTokens + out.CachedTokens
	return out, nil
}

var (
	ErrNotFound  = errors.New("payload not found")
	ErrNoContent = errors.New("no content")
)

// GetPayload returns raw payload bytes and a content-type. It does not return metadata.
func (s *Service) GetPayload(ctx context.Context, id string) ([]byte, string, error) {
	if s == nil || s.store == nil || strings.TrimSpace(id) == "" {
		return nil, "", ErrNotFound
	}
	rows, err := s.store.Payloads().List(ctx, plread.WithID(id))
	if err != nil {
		return nil, "", err
	}
	if len(rows) == 0 || rows[0] == nil {
		return nil, "", ErrNotFound
	}
	v := rows[0]
	if v.InlineBody == nil || len(*v.InlineBody) == 0 {
		return nil, "", ErrNoContent
	}
	ctype := v.MimeType
	if strings.TrimSpace(ctype) == "" {
		ctype = "application/octet-stream"
	}
	return *v.InlineBody, ctype, nil
}

// Stage exposes currentStage for external callers.
func (s *Service) Stage(ctx context.Context, conversationID string) *stage.Stage {
	return s.currentStage(ctx, conversationID)
}
