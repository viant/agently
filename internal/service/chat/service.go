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
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/conversation"
	cancels "github.com/viant/agently/genai/conversation/cancel"
	"github.com/viant/agently/genai/elicitation"
	promptpkg "github.com/viant/agently/genai/prompt"
	agentpkg "github.com/viant/agently/genai/service/agent"
	"github.com/viant/agently/genai/tool"
	authctx "github.com/viant/agently/internal/auth"
	implconv "github.com/viant/agently/internal/service/conversation"
	convw "github.com/viant/agently/pkg/agently/conversation/write"
	msgwrite "github.com/viant/agently/pkg/agently/message/write"
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

// GetRequest defines inputs to fetch messages.
type GetRequest struct {
	ConversationID          string
	IncludeModelCallPayload bool
	SinceID                 string // optional: inclusive slice starting from this message id
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
	conv, err := s.convClient.GetConversation(ctx, req.ConversationID, opts...)
	if err != nil {
		return nil, err
	}
	return &GetResponse{Conversation: conv}, nil
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
			fmt.Fprintf(os.Stderr, "failed to process message: 	%v\n", err)
			if s.convClient != nil {
				tUpd := apiconv.NewTurn()
				tUpd.SetId(msgID)
				tUpd.SetStatus("failed")
				tUpd.SetErrorMessage(err.Error())
				_ = s.convClient.PatchTurn(runCtx, tUpd)
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
