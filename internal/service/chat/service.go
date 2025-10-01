package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/viant/afs"
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/conversation"
	cancels "github.com/viant/agently/genai/conversation/cancel"
	"github.com/viant/agently/genai/elicitation"
	execcfg "github.com/viant/agently/genai/executor/config"
	"github.com/viant/agently/genai/llm"
	promptpkg "github.com/viant/agently/genai/prompt"
	agentpkg "github.com/viant/agently/genai/service/agent"
	corellm "github.com/viant/agently/genai/service/core"
	"github.com/viant/agently/genai/tool"
	authctx "github.com/viant/agently/internal/auth"
	extrepo "github.com/viant/agently/internal/repository/extension"
	implconv "github.com/viant/agently/internal/service/conversation"
	convw "github.com/viant/agently/pkg/agently/conversation/write"
	msgwrite "github.com/viant/agently/pkg/agently/message/write"
	toolfeed "github.com/viant/agently/pkg/agently/tool"
	mcptool "github.com/viant/fluxor-mcp/mcp/tool"
	fluxpol "github.com/viant/fluxor/policy"
	"github.com/viant/fluxor/service/approval"
	fservice "github.com/viant/forge/backend/service/file"
)

// Service exposes message retrieval independent of HTTP concerns.
type Service struct {
	mgr        *conversation.Manager
	toolPolicy *tool.Policy
	fluxPolicy *fluxpol.Policy
	approval   approval.Service

	convClient apiconv.Client
	fileSvc    *fservice.Service

	elicitation *elicitation.Service
	reg         cancels.Registry

	core     *corellm.Service
	defaults *execcfg.Defaults
}

// API defines the minimal interface the HTTP layer depends on. It allows
// substituting the concrete chat service with a mock or alternative impl
// without coupling the handler to the struct type.
type API interface {
	// Wiring hooks (used during server bootstrap)
	AttachManager(mgr *conversation.Manager, tp *tool.Policy, fp *fluxpol.Policy)
	AttachApproval(svc approval.Service)
	AttachFileService(fs *fservice.Service)
	AttachElicitationService(es *elicitation.Service)
	AttachCore(core *corellm.Service)
	AttachDefaults(d *execcfg.Defaults)

	// Core operations
	Get(ctx context.Context, req GetRequest) (*GetResponse, error)
	MatchToolFeedSpec(ctx context.Context, conversationID, sinceID string) ([]*toolfeed.FeedSpec, error)
	Generate(ctx context.Context, input *corellm.GenerateInput) (*corellm.GenerateOutput, error)
}

func NewService() *Service {
	svc := &Service{reg: cancels.Default()}
	if dao, err := implconv.NewDatly(context.Background()); err == nil {
		if cli, err := implconv.New(context.Background(), dao); err == nil {
			svc.convClient = cli
		}
	}
	return svc
}

// AttachElicitationService wires the elicitation service to avoid ad-hoc constructions.
func (s *Service) AttachElicitationService(es *elicitation.Service) { s.elicitation = es }
func (s *Service) ElicitationService() *elicitation.Service         { return s.elicitation }

// ResumeElicitation triggers agent processing for a conversation after an
// elicitation has been accepted and payload stored. It starts a new turn and
// lets the agent continue based on the updated conversation state.
// ResumeElicitation removed – resumption is coordinated by the agent loop via router wait.

// ConversationClient exposes the underlying conversation client for handlers that need
// fine-grained operations without adding more methods to this service.
func (s *Service) ConversationClient() apiconv.Client { return s.convClient }

// AttachManager configures the conversation manager and optional default policies.
func (s *Service) AttachManager(mgr *conversation.Manager, tp *tool.Policy, fp *fluxpol.Policy) {
	s.mgr = mgr
	s.toolPolicy = tp
	s.fluxPolicy = fp
}

// AttachApproval configures the approval service bridge for policy decisions.
func (s *Service) AttachApproval(svc approval.Service) { s.approval = svc }

// AttachFileService wires the Forge file service instance so that attachment
// reads can reuse the same staging root and resolution.
func (s *Service) AttachFileService(fs *fservice.Service) { s.fileSvc = fs }
func (s *Service) AttachCore(core *corellm.Service)       { s.core = core }
func (s *Service) AttachDefaults(d *execcfg.Defaults)     { s.defaults = d }

// GetRequest defines inputs to fetch messages.
type GetRequest struct {
	ConversationID          string
	IncludeModelCallPayload bool
	SinceID                 string // optional: inclusive slice starting from this message id
	IncludeToolCall         bool
	ToolExtensions          []*toolfeed.FeedSpec
}

// GetResponse carries the rich conversation view for the given request.
type GetResponse struct {
	Conversation *apiconv.Conversation
}

// Get fetches a conversation using the rich transcript API.
func (s *Service) Get(ctx context.Context, req GetRequest) (*GetResponse, error) {
	var opts []apiconv.Option
	if id := strings.TrimSpace(req.SinceID); id != "" {
		opts = append(opts, apiconv.WithSince(id))
	}
	if req.IncludeModelCallPayload {
		opts = append(opts, apiconv.WithIncludeModelCall(true))
	}
	if req.IncludeToolCall {
		opts = append(opts, apiconv.WithIncludeToolCall(true))
	}
	if len(req.ToolExtensions) > 0 {
		opts = append(opts, apiconv.WithToolFeedSpec(req.ToolExtensions))
	}
	conv, err := s.convClient.GetConversation(ctx, req.ConversationID, opts...)
	if err != nil {
		return nil, err
	}
	return &GetResponse{Conversation: conv}, nil
}

// Generate exposes low-level core LLM generate bypassing agentic enrichment.
func (s *Service) Generate(ctx context.Context, input *corellm.GenerateInput) (*corellm.GenerateOutput, error) {
	if s == nil || s.core == nil || input == nil {
		return &corellm.GenerateOutput{}, nil
	}
	var out corellm.GenerateOutput
	if err := s.core.Generate(ctx, input, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ResolveToolExtensions computes applicable ToolExtensions for a conversation
// by inspecting observed tool calls and matching them against workspace
// extensions. When no tools are observed it includes any extensions with
// activation.kind==tool_call so that the hook can invoke them live.
func (s *Service) MatchToolFeedSpec(ctx context.Context, conversationID, sinceID string) ([]*toolfeed.FeedSpec, error) {
	if s == nil || s.convClient == nil {
		return nil, nil
	}
	// Fetch minimal conversation view including tool calls
	var opts []apiconv.Option
	if strings.TrimSpace(sinceID) != "" {
		opts = append(opts, apiconv.WithSince(sinceID))
	}
	opts = append(opts, apiconv.WithIncludeToolCall(true))
	conv, err := s.convClient.GetConversation(ctx, conversationID, opts...)
	if err != nil || conv == nil {
		return nil, err
	}
	// Collect observed tool names
	services := map[string]struct{}{}
	if tr := conv.GetTranscript(); tr != nil {
		for _, name := range tr.UniqueToolNames() {
			services[name] = struct{}{}
		}
	}
	repo := extrepo.New(afs.New())
	seen := map[string]struct{}{}
	var result []*toolfeed.FeedSpec
	// Match by observed tools
	for service := range services {
		canonical := mcptool.Canonical(service)
		name := mcptool.Name(canonical)
		svc := name.Service()
		method := name.Method()

		matched, _ := repo.FindMatches(ctx, svc, method)
		for _, e := range matched {
			if _, ok := seen[e.ID]; ok {
				continue
			}
			seen[e.ID] = struct{}{}
			result = append(result, (*toolfeed.FeedSpec)(e))
		}
	}

	// When no tools observed, include tool_call extensions explicitly
	if len(services) == 0 {
		if names, err := repo.List(ctx); err == nil {
			for _, n := range names {
				rec, err := repo.Load(ctx, n)
				if err != nil || rec == nil {
					continue
				}
				if strings.EqualFold(strings.TrimSpace(rec.Activation.Kind), "tool_call") {
					if _, ok := seen[rec.ID]; ok {
						continue
					}
					seen[rec.ID] = struct{}{}
					result = append(result, (*toolfeed.FeedSpec)(rec))
				}
			}
		}
	}
	return result, nil
}

// PostRequest defines inputs to submit a user message.
type PostRequest struct {
	Content string                 `json:"content"`
	Agent   string                 `json:"agent,omitempty"`
	Model   string                 `json:"model,omitempty"`
	Tools   []string               `json:"tools,omitempty"`
	Context map[string]interface{} `json:"context,omitempty"`
	// Attachments carries staged upload descriptors returned by Forge upload endpoint.
	// Each item must include at least name and uri (relative to storage root), optionally size, stagingFolder, mime.
	Attachments []UploadedAttachment `json:"attachments,omitempty"`
}

// UploadedAttachment mirrors Forge upload response structure.
type UploadedAttachment struct {
	Name          string `json:"name"`
	Size          int    `json:"size,omitempty"`
	StagingFolder string `json:"stagingFolder,omitempty"`
	URI           string `json:"uri"`
	Mime          string `json:"mime,omitempty"`
}

// PreflightPost validates minimal conditions before accepting a post.
// It ensures an agent can be determined either from request or conversation defaults.
func (s *Service) PreflightPost(ctx context.Context, conversationID string, req PostRequest) error {
	if strings.TrimSpace(req.Agent) != "" {
		return nil
	}
	// Check conversation has AgentName
	if s.convClient != nil {
		cv, err := s.convClient.GetConversation(ctx, conversationID)
		if err != nil {
			return err
		}
		if cv != nil && cv.AgentName != nil && strings.TrimSpace(*cv.AgentName) != "" {
			return nil
		}
	}
	return fmt.Errorf("agent is required")
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

	// Launch asynchronous processing to avoid blocking HTTP caller.
	go func(parent context.Context) {
		// Detach from HTTP cancellation but preserve auth and policies.
		base := context.Background()
		// Preserve auth bearer and user info if present
		if ui := authctx.User(parent); ui != nil {
			base = authctx.WithUserInfo(base, ui)
		}
		if tok := authctx.Bearer(parent); tok != "" {
			base = authctx.WithBearer(base, tok)
		}
		runCtx, cancel := context.WithCancel(base)
		if s.reg != nil {
			s.reg.Register(conversationID, msgID, cancel)
			defer s.reg.Complete(conversationID, msgID, cancel)
		} else {
			defer cancel()
		}

		// Convert staged uploads into attachments (read + cleanup)
		s.enrichAttachmentIfNeeded(req, runCtx, input)

		// Propagate conversation ID and policies
		runCtx = conversation.WithID(runCtx, conversationID)
		if s.toolPolicy != nil {
			runCtx = tool.WithPolicy(runCtx, s.toolPolicy)
		} else {
			runCtx = tool.WithPolicy(runCtx, &tool.Policy{Mode: tool.ModeAuto})
		}
		if pol := tool.FromContext(parent); pol != nil {
			runCtx = tool.WithPolicy(runCtx, pol)
		}
		if s.fluxPolicy != nil {
			runCtx = fluxpol.WithPolicy(runCtx, s.fluxPolicy)
		}
		// Execute agentic flow; turn/message persistence handled by agent recorder.
		_, err := s.mgr.Accept(runCtx, input)
		if err != nil {
			if s.convClient != nil {
				tUpd := apiconv.NewTurn()
				tUpd.SetId(msgID)
				if errors.Is(err, context.Canceled) {
					// Persist canceled using background context; avoid writing with canceled ctx
					tUpd.SetStatus("canceled")
					_ = s.convClient.PatchTurn(context.Background(), tUpd)
				} else {
					tUpd.SetStatus("failed")
					tUpd.SetErrorMessage(err.Error())
					_ = s.convClient.PatchTurn(runCtx, tUpd)
				}
			}
		}
	}(ctx)

	return msgID, nil
}

func (s *Service) enrichAttachmentIfNeeded(req PostRequest, runCtx context.Context, input *agentpkg.QueryInput) error {
	if len(req.Attachments) == 0 {
		return nil
	}
	// build list and folders to cleanup
	folders := map[string]struct{}{}
	for _, a := range req.Attachments {
		uri := strings.TrimSpace(a.URI)
		if uri == "" {
			continue
		}
		data, err := s.fileSvc.Download(runCtx, a.URI)
		if err != nil {
			return fmt.Errorf("download attachment: %w", err)
		}

		if a.StagingFolder == "" {
			a.StagingFolder, _ = path.Split(uri)
		}
		name := strings.TrimSpace(a.Name)
		// Determine MIME: prefer provided, else sniff content, else extension (built-in)
		mimeType := strings.TrimSpace(a.Mime)
		if mimeType == "" {
			mimeType = mime.TypeByExtension(filepath.Ext(a.Name))
		}
		att := &promptpkg.Attachment{
			Name:    name,
			URI:     uri,
			Mime:    mimeType,
			Content: "",
			Data:    data,
		}
		input.Attachments = append(input.Attachments, att)
		// best-effort delete file
		// best-effort cleanup is handled by file service lifecycle
		if folder := strings.TrimSpace(a.StagingFolder); folder != "" {
			folders[folder] = struct{}{}
		}
	}
	// cleanup empty folders best-effort
	for folder := range folders {
		clean := strings.TrimPrefix(folder, "/")
		_ = os.Remove(filepath.Clean(clean))
	}

	return nil
}

// Cancel aborts all in-flight turns for the given conversation; returns true if any were cancelled.
func (s *Service) Cancel(conversationID string) bool {
	if s == nil || s.reg == nil {
		return false
	}
	return s.reg.CancelConversation(conversationID)
}

// CancelTurn aborts a specific user turn (keyed by messageId) if running.
func (s *Service) CancelTurn(turnID string) bool {
	if s == nil || s.reg == nil {
		return false
	}
	return s.reg.CancelTurn(turnID)
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
	if err := s.convClient.PatchConversations(ctx, (*apiconv.MutableConversation)(cw)); err != nil {
		return nil, fmt.Errorf("failed to persist conversation: %w", err)
	}
	return &CreateConversationResponse{ID: id, Title: title, CreatedAt: createdAt.Format(time.RFC3339), Model: in.Model, Agent: in.Agent, Tools: in.Tools}, nil
}

// GetConversation returns id + title by conversation id.
func (s *Service) GetConversation(ctx context.Context, id string) (*ConversationSummary, error) {
	cv, err := s.convClient.GetConversation(ctx, id)
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
	rows, err := s.convClient.GetConversations(ctx)
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

// DeleteConversation removes a conversation and cascades to dependent rows via DB FKs.
// It delegates to the underlying conversation client implementation.
func (s *Service) DeleteConversation(ctx context.Context, id string) error {
	if s == nil || s.convClient == nil || strings.TrimSpace(id) == "" {
		return nil
	}
	type deleter interface {
		DeleteConversation(context.Context, string) error
	}
	if d, ok := s.convClient.(deleter); ok {
		return d.DeleteConversation(ctx, id)
	}
	return fmt.Errorf("delete not supported")
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
	case "accept", "accepted", "approve", "approved", "yes", "y", "decline", "denied", "deny", "reject", "rejected", "no", "n":
		// proceed
	default:
		return fmt.Errorf("invalid action")
	}

	// Map to status and approved flag
	approved := action == "accept" || action == "accepted" || action == "approve" || action == "approved" || action == "yes" || action == "y"
	newStatus := "rejected"
	if approved {
		newStatus = "accepted"
	}

	m := &msgwrite.Message{Id: messageID, Status: newStatus, Has: &msgwrite.MessageHas{Status: true}}
	_ = s.convClient.PatchMessage(ctx, (*apiconv.MutableMessage)(m))

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
	if s == nil || s.convClient == nil || s.elicitation == nil {
		return fmt.Errorf("elicitation service not configured")
	}
	elicitationMsg, err := s.convClient.GetMessage(ctx, messageID)
	if err != nil {
		return err
	}
	if elicitationMsg == nil {
		return fmt.Errorf("elicitation message not found")
	}
	// Always resolve via elicitation service; it patches status in all cases and stores payload when accepted
	if err := s.elicitation.Resolve(ctx, elicitationMsg.ConversationId, *elicitationMsg.ElicitationId, action, payload); err != nil {
		return err
	}
	return nil
}

var (
	ErrNotFound  = errors.New("payload not found")
	ErrNoContent = errors.New("no content")
)

// GetPayload returns raw payload bytes and a content-type. It does not return metadata.
func (s *Service) GetPayload(ctx context.Context, id string) ([]byte, string, error) {
	if s == nil || strings.TrimSpace(id) == "" {
		return nil, "", ErrNotFound
	}
	p, err := s.convClient.GetPayload(ctx, id)
	if err != nil || p == nil {
		return nil, "", ErrNotFound
	}
	if p.InlineBody == nil || len(*p.InlineBody) == 0 {
		return nil, "", ErrNoContent
	}
	ctype := p.MimeType
	if strings.TrimSpace(ctype) == "" {
		ctype = "application/octet-stream"
	}
	return *p.InlineBody, ctype, nil
}

// ---- Status helpers (implements chat.Client) ----

// SetTurnStatus patches a turn status. Use context.Background() when invoked from cancel path.
func (s *Service) SetTurnStatus(ctx context.Context, turnID, status string, errorMessage ...string) error {
	if s == nil || s.convClient == nil || strings.TrimSpace(turnID) == "" || strings.TrimSpace(status) == "" {
		return nil
	}
	upd := apiconv.NewTurn()
	upd.SetId(turnID)
	upd.SetStatus(status)
	if len(errorMessage) > 0 && strings.TrimSpace(errorMessage[0]) != "" {
		upd.SetErrorMessage(errorMessage[0])
	}
	return s.convClient.PatchTurn(ctx, upd)
}

// SetMessageStatus patches a message status.
func (s *Service) SetMessageStatus(ctx context.Context, messageID, status string) error {
	if s == nil || s.convClient == nil || strings.TrimSpace(messageID) == "" || strings.TrimSpace(status) == "" {
		return nil
	}
	upd := apiconv.NewMessage()
	upd.SetId(messageID)
	upd.SetStatus(status)
	return s.convClient.PatchMessage(ctx, upd)
}

// SetConversationStatus updates the latest turn in a conversation.
func (s *Service) SetConversationStatus(ctx context.Context, conversationID, status string) error {
	if s == nil || s.convClient == nil || strings.TrimSpace(conversationID) == "" || strings.TrimSpace(status) == "" {
		return nil
	}
	conv, err := s.convClient.GetConversation(ctx, conversationID)
	if err != nil || conv == nil {
		return err
	}
	tr := conv.GetTranscript()
	if len(tr) == 0 || tr[len(tr)-1] == nil {
		return nil
	}
	last := tr[len(tr)-1]
	// 1) Update turn status first
	if err := s.SetTurnStatus(ctx, last.Id, status); err != nil {
		return err
	}
	// 2) Best-effort: update last assistant interim/message status to mirror termination
	//    (helps downstream stage derivation when DB view considers message status)
	msgs := last.Message
	for i := len(msgs) - 1; i >= 0; i-- { // scan backwards to catch latest assistant
		m := msgs[i]
		if m == nil {
			continue
		}
		role := strings.ToLower(m.Role)
		if role == "assistant" {
			// guard: id must be present
			if strings.TrimSpace(m.Id) != "" {
				_ = s.SetMessageStatus(ctx, m.Id, status)
			}
			break
		}
	}
	return nil
}

// Compact creates a summary message and flags prior messages as compacted (excluding elicitation I/O).
func (s *Service) Compact(ctx context.Context, conversationID string) error {
	if s == nil || s.convClient == nil || strings.TrimSpace(conversationID) == "" {
		return nil
	}
	conv, err := s.convClient.GetConversation(ctx, conversationID)
	if err != nil || conv == nil {
		return err
	}
	tr := conv.GetTranscript()
	// Try LLM-generated summary via manager first
	var msgID string
	if s.mgr != nil {
		if id, lerr := s.compactGenerateSummaryLLM(ctx, conv); lerr == nil && strings.TrimSpace(id) != "" {
			msgID = id
		}
	}
	if strings.TrimSpace(msgID) == "" {
		summary := s.compactBuildSummary(tr)
		id, ierr := s.compactInsertSummary(ctx, conversationID, summary)
		if ierr != nil {
			return ierr
		}
		msgID = id
	}
	s.compactFlagPrior(ctx, tr, msgID)
	return nil
}

// compactBuildSummary collects labelled user/assistant snippets (newest→oldest, limited) and reverses them.
func (s *Service) compactBuildSummary(tr apiconv.Transcript) string {
	const maxSnippets = 16
	var out []string
	for ti := len(tr) - 1; ti >= 0 && len(out) < maxSnippets; ti-- {
		t := tr[ti]
		if t == nil || len(t.Message) == 0 {
			continue
		}
		for mi := len(t.Message) - 1; mi >= 0 && len(out) < maxSnippets; mi-- {
			m := t.Message[mi]
			if m == nil || m.Interim != 0 {
				continue
			}
			if m.ElicitationId != nil && strings.TrimSpace(*m.ElicitationId) != "" {
				continue
			}
			role := strings.ToLower(strings.TrimSpace(m.Role))
			if role != "user" && role != "assistant" {
				continue
			}
			if m.Content == nil {
				continue
			}
			c := strings.TrimSpace(*m.Content)
			if c == "" {
				continue
			}
			label := "User"
			if role == "assistant" {
				label = "Assistant"
			}
			out = append(out, label+": "+c)
		}
	}
	if len(out) == 0 {
		return "Conversation summary (auto-generated)."
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return strings.Join(out, "\n\n")
}

// compactInsertSummary writes a single assistant summary message and returns its id.
func (s *Service) compactInsertSummary(ctx context.Context, conversationID, summary string) (string, error) {
	msgID := uuid.NewString()
	mm := apiconv.NewMessage()
	mm.SetId(msgID)
	mm.SetConversationID(conversationID)
	mm.SetRole("assistant")
	mm.SetType("text")
	mm.SetStatus("summary")
	mm.SetContent(summary)
	return msgID, s.convClient.PatchMessage(ctx, mm)
}

// compactFlagPrior sets compacted=1 on all prior messages except elicitation and excludeMsgID.
func (s *Service) compactFlagPrior(ctx context.Context, tr apiconv.Transcript, excludeMsgID string) {
	for _, t := range tr {
		if t == nil || len(t.Message) == 0 {
			continue
		}
		for _, m := range t.Message {
			if m == nil {
				continue
			}
			if strings.TrimSpace(m.Id) == strings.TrimSpace(excludeMsgID) {
				continue
			}
			if m.ElicitationId != nil && strings.TrimSpace(*m.ElicitationId) != "" {
				continue
			}
			upd := apiconv.NewMessage()
			upd.SetId(m.Id)
			upd.SetCompacted(1)
			_ = s.convClient.PatchMessage(ctx, upd)
		}
	}
}

// compactGenerateSummaryLLM performs a one-off LLM turn to summarize the conversation and returns the assistant message id.
func (s *Service) compactGenerateSummaryLLM(ctx context.Context, conv *apiconv.Conversation) (string, error) {
	if s == nil || conv == nil {
		return "", fmt.Errorf("llm unavailable")
	}
	// Prefer direct core.Generate to bypass agent enrichment
	if s.core != nil {
		tr := conv.GetTranscript()
		latest := s.compactLatestMessageID(tr)
		var msgs []llm.Message
		// Determine slice size from defaults
		maxN := 30
		if s.defaults != nil && s.defaults.SummaryLastN > 0 {
			maxN = s.defaults.SummaryLastN
		}
		count := 0
		for ti := len(tr) - 1; ti >= 0 && count < maxN; ti-- {
			t := tr[ti]
			if t == nil || len(t.Message) == 0 {
				continue
			}
			for mi := len(t.Message) - 1; mi >= 0 && count < maxN; mi-- {
				m := t.Message[mi]
				if m == nil || m.Interim != 0 {
					continue
				}
				if strings.TrimSpace(m.Id) == strings.TrimSpace(latest) {
					continue
				}
				if m.ElicitationId != nil && strings.TrimSpace(*m.ElicitationId) != "" {
					continue
				}
				role := strings.ToLower(strings.TrimSpace(m.Role))
				if role != "user" && role != "assistant" {
					continue
				}
				if m.Content == nil || strings.TrimSpace(*m.Content) == "" {
					continue
				}
				msgs = append(msgs, llm.NewTextMessage(llm.MessageRole(role), *m.Content))
				count++
			}
		}
		instruction := "Summarize key points to continue the discussion. Be concise (<=6 bullets), include goals/decisions/next steps, avoid logs/quotes. Exclude the latest message."
		if s.defaults != nil && strings.TrimSpace(s.defaults.SummaryPrompt) != "" {
			instruction = strings.TrimSpace(s.defaults.SummaryPrompt)
		}
		msgs = append(msgs, llm.NewUserMessage(instruction))
		model := ""
		if s.defaults != nil && strings.TrimSpace(s.defaults.SummaryModel) != "" {
			model = strings.TrimSpace(s.defaults.SummaryModel)
		} else if conv.DefaultModel != nil && strings.TrimSpace(*conv.DefaultModel) != "" {
			model = strings.TrimSpace(*conv.DefaultModel)
		}
		in := &corellm.GenerateInput{ModelSelection: llm.ModelSelection{Model: model}, Message: msgs}
		var out corellm.GenerateOutput
		if err := s.core.Generate(ctx, in, &out); err != nil {
			return "", err
		}
		content := strings.TrimSpace(out.Content)
		if content == "" {
			return "", fmt.Errorf("empty summary")
		}
		id, ierr := s.compactInsertSummary(ctx, conv.Id, content)
		if ierr != nil {
			return "", ierr
		}
		return id, nil
	}
	// Fallback to agent manager Accept path
	if s.mgr == nil {
		return "", fmt.Errorf("llm unavailable")
	}
	agent := "chat"
	if conv.AgentName != nil && strings.TrimSpace(*conv.AgentName) != "" {
		agent = strings.TrimSpace(*conv.AgentName)
	}
	prompt := "Summarize the conversation so far into key points to support continuing the discussion.\n- Be concise (<= 6 bullets) and actionable.\n- Include user goals, decisions, and pending next steps.\n- Avoid long quotes/logs; describe results briefly.\n- Exclude the latest user or assistant message from the summary; summarize prior context only.\n"
	in2 := &agentpkg.QueryInput{ConversationID: conv.Id, Query: prompt, AgentName: agent, ToolsAllowed: []string{}}
	if _, err := s.mgr.Accept(ctx, in2); err != nil {
		return "", err
	}
	updated, err := s.convClient.GetConversation(ctx, conv.Id)
	if err != nil || updated == nil {
		return "", err
	}
	tr2 := updated.GetTranscript()
	if len(tr2) == 0 || tr2[len(tr2)-1] == nil {
		return "", fmt.Errorf("no turn created")
	}
	last := tr2[len(tr2)-1]
	for i := len(last.Message) - 1; i >= 0; i-- {
		m := last.Message[i]
		if m == nil || m.Interim != 0 {
			continue
		}
		if strings.ToLower(strings.TrimSpace(m.Role)) == "assistant" && strings.TrimSpace(m.Id) != "" {
			_ = s.SetMessageStatus(ctx, m.Id, "summary")
			return m.Id, nil
		}
	}
	return "", fmt.Errorf("assistant summary not found")
}

// compactLatestMessageID returns the newest non-interim message id in the transcript (or empty when none).
func (s *Service) compactLatestMessageID(tr apiconv.Transcript) string {
	for ti := len(tr) - 1; ti >= 0; ti-- {
		t := tr[ti]
		if t == nil || len(t.Message) == 0 {
			continue
		}
		for mi := len(t.Message) - 1; mi >= 0; mi-- {
			m := t.Message[mi]
			if m == nil || m.Interim != 0 {
				continue
			}
			if id := strings.TrimSpace(m.Id); id != "" {
				return id
			}
		}
	}
	return ""
}
