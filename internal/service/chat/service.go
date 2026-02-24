package chat

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/viant/afs"
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/agent"
	genconv "github.com/viant/agently/genai/conversation"
	cancels "github.com/viant/agently/genai/conversation/cancel"
	"github.com/viant/agently/genai/elicitation"
	execcfg "github.com/viant/agently/genai/executor/config"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/memory"
	promptpkg "github.com/viant/agently/genai/prompt"
	agentpkg "github.com/viant/agently/genai/service/agent"
	agentsrv "github.com/viant/agently/genai/service/agent"
	"github.com/viant/agently/genai/service/agent/prompts"
	corellm "github.com/viant/agently/genai/service/core"
	"github.com/viant/agently/genai/service/shared"
	"github.com/viant/agently/genai/tool"
	msgsvc "github.com/viant/agently/genai/tool/service/message"
	approval "github.com/viant/agently/internal/approval"
	authctx "github.com/viant/agently/internal/auth"
	implconv "github.com/viant/agently/internal/service/conversation"
	usersvc "github.com/viant/agently/internal/service/user"
	extrepo "github.com/viant/agently/internal/workspace/repository/extension"
	agconv "github.com/viant/agently/pkg/agently/conversation"
	convw "github.com/viant/agently/pkg/agently/conversation/write"
	msgwrite "github.com/viant/agently/pkg/agently/message/write"
	toolfeed "github.com/viant/agently/pkg/agently/tool"
	turnread "github.com/viant/agently/pkg/agently/turn/read"
	oauthread "github.com/viant/agently/pkg/agently/user/oauth"
	oauthwrite "github.com/viant/agently/pkg/agently/user/oauth/write"
	mcpname "github.com/viant/agently/pkg/mcpname"
	component "github.com/viant/agently/pkg/service"
	"github.com/viant/datly"
	"github.com/viant/datly/repository/contract"
	fservice "github.com/viant/forge/backend/service/file"
	scyauth "github.com/viant/scy/auth"
	xhttp "github.com/viant/xdatly/handler/http"
	"github.com/viant/xdatly/handler/state"
	"golang.org/x/oauth2"
)

//go:embed compact.md
var compactInstruction string

const (
	pruneMinRemove = 20
	pruneMaxRemove = 50
)

// Service exposes message retrieval independent of HTTP concerns.
type Service struct {
	mgr        *genconv.Manager
	toolPolicy *tool.Policy
	approval   approval.Service
	authCfg    *authctx.Config

	convClient apiconv.Client
	fileSvc    *fservice.Service

	elicitation *elicitation.Service
	reg         cancels.Registry
	agentFinder agent.Finder

	core     *corellm.Service
	defaults *execcfg.Defaults
	// Optional: user preferences loader
	users *usersvc.Service
	dao   *datly.Service

	activeTurnSvc *component.Service[turnread.ActiveTurnInput, turnread.ActiveTurnOutput]
	nextQueuedSvc *component.Service[turnread.NextQueuedInput, turnread.NextQueuedOutput]

	queueMu   sync.Mutex
	queueByID map[string]*conversationQueue
}

type conversationQueue struct {
	notify chan struct{}
}

//// API defines the minimal interface the HTTP layer depends on. It allows
//// substituting the concrete chat service with a mock or alternative impl
//// without coupling the handler to the struct type.
//type API interface {
//	// Wiring hooks (used during server bootstrap)
//	AttachManager(mgr *conversation.Manager, tp *tool.Policy, fp *fluxpol.Policy)
//	AttachApproval(svc approval.Service)
//	AttachFileService(fs *fservice.Service)
//	AttachElicitationService(es *elicitation.Service)
//	AttachCore(core *corellm.Service)
//	AttachDefaults(d *execcfg.Defaults)
//
//	// Core operations
//	Get(ctx context.Context, req GetRequest) (*GetResponse, error)
//	MatchToolFeedSpec(ctx context.Context, conversationID, sinceID string) ([]*toolfeed.FeedSpec, error)
//	Generate(ctx context.Context, input *corellm.GenerateInput) (*corellm.GenerateOutput, error)
//}

func NewService() *Service {
	// Legacy constructor: keep silent auto-wiring for backwards compatibility.
	svc := &Service{reg: cancels.Default()}
	if dao, err := implconv.NewDatly(context.Background()); err == nil {
		// Register OAuth token components best-effort; legacy constructor suppresses errors
		_ = oauthread.DefineTokenComponent(context.Background(), dao)
		_, _ = oauthwrite.DefineComponent(context.Background(), dao)
		if cli, err := implconv.New(context.Background(), dao); err == nil {
			svc.convClient = cli
			svc.dao = dao
		}
	}
	return svc
}

// NewServiceFromEnv wires conversation client using environment configuration.
// Returns an error when conversation client cannot be initialized so that
// callers can fail fast instead of starting partially configured services.
func NewServiceFromEnv(ctx context.Context) (*Service, error) {
	dao, err := implconv.NewDatly(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to init datly: %w", err)
	}
	// Ensure OAuth token components are registered on this DAO so token store DAO can Operate
	// using the same service (e.g., server-side EnsureToken during post message).
	if err := oauthread.DefineTokenComponent(ctx, dao); err != nil {
		return nil, fmt.Errorf("failed to define oauth token read component: %w", err)
	}
	if _, err := oauthwrite.DefineComponent(ctx, dao); err != nil {
		return nil, fmt.Errorf("failed to define oauth token write component: %w", err)
	}
	cli, err := implconv.New(ctx, dao)
	if err != nil {
		return nil, fmt.Errorf("failed to init conversation client: %w", err)
	}
	// Try to wire user preferences service (best-effort)
	var users *usersvc.Service
	if u, uerr := usersvc.New(ctx, dao); uerr == nil {
		users = u
	}
	svc := &Service{reg: cancels.Default(), convClient: cli, users: users, dao: dao}
	return svc, nil
}

// NewServiceWithClient constructs a chat service bound to a provided conversation client and shared datly.
func NewServiceWithClient(cli apiconv.Client, dao *datly.Service) *Service {
	return &Service{reg: cancels.Default(), convClient: cli, dao: dao}
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
func (s *Service) AttachManager(mgr *genconv.Manager, tp *tool.Policy) {
	s.mgr = mgr
	s.toolPolicy = tp
}

// AttachApproval configures the approval service bridge for policy decisions.
func (s *Service) AttachApproval(svc approval.Service)  { s.approval = svc }
func (s *Service) AttachAuthConfig(cfg *authctx.Config) { s.authCfg = cfg }

// AttachFileService wires the Forge file service instance so that attachment
// reads can reuse the same staging root and resolution.
func (s *Service) AttachFileService(fs *fservice.Service) { s.fileSvc = fs }
func (s *Service) AttachCore(core *corellm.Service)       { s.core = core }
func (s *Service) AttachDefaults(d *execcfg.Defaults)     { s.defaults = d }

func (s *Service) AttacheAgentFinder(agentFinder agent.Finder) { s.agentFinder = agentFinder }

// DAO returns the underlying datly service when available.
func (s *Service) DAO() *datly.Service { return s.dao }

// UserByUsername returns user id when available.
func (s *Service) UserByUsername(ctx context.Context, username string) (string, error) {
	if s.users == nil {
		return "", fmt.Errorf("user service not initialised")
	}
	v, err := s.users.FindByUsername(ctx, username)
	if err != nil {
		return "", err
	}
	if v == nil {
		return "", fmt.Errorf("user not found")
	}
	return v.Id, nil
}

// Query executes a synchronous agent turn using the configured conversation manager.
// It bubbles up any errors from downstream services without printing.
func (s *Service) Query(ctx context.Context, input *agentpkg.QueryInput) (*agentpkg.QueryOutput, error) {
	if s == nil || s.mgr == nil {
		return nil, fmt.Errorf("conversation manager is not configured")
	}
	if input == nil {
		return nil, fmt.Errorf("input is nil")
	}
	// If a default tool policy is configured and none is present in the context,
	// attach it to guide tool execution behaviour. Do not override existing.
	if s.toolPolicy != nil && tool.FromContext(ctx) == nil {
		ctx = tool.WithPolicy(ctx, s.toolPolicy)
	}
	// Delegate to Manager.Accept which ensures conversation id and runs the agent flow.
	return s.mgr.Accept(ctx, input)
}

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
	// Service invariant: endpoints are only registered when convClient is configured.
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

// DeleteMessage removes a message from a conversation via the underlying client.
func (s *Service) DeleteMessage(ctx context.Context, convID, messageID string) error {
	if s == nil || s.convClient == nil {
		return fmt.Errorf("conversation client is not configured")
	}
	return s.convClient.DeleteMessage(ctx, convID, messageID)
}

// PrepareToolContext enriches args with conversation context (conversationId, lastTurnId)
// and returns a context carrying the conversationId for downstream services.
// convID must be non-empty.
func (s *Service) PrepareToolContext(ctx context.Context, convID string, args map[string]interface{}) (context.Context, map[string]interface{}, error) {
	if strings.TrimSpace(convID) == "" {
		return ctx, nil, fmt.Errorf("conversation id is required")
	}
	// Enrich context with conversation id
	ctx = memory.WithConversationID(ctx, convID)

	// Best-effort resolve last turn id
	lastTurnID := ""
	if s != nil && s.convClient != nil {
		if conv, err := s.convClient.GetConversation(ctx, convID); err == nil && conv != nil {
			lastTurnID = *conv.LastTurnId
		}
	}
	ctx = memory.WithConversationID(ctx, convID)
	ctx = memory.WithTurnMeta(ctx, memory.TurnMeta{
		ConversationID:  convID,
		TurnID:          lastTurnID,
		ParentMessageID: lastTurnID,
	})
	return ctx, args, nil
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
		canonical := mcpname.Canonical(service)
		name := mcpname.Name(canonical)
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
	Tools   []string               `json:"tools"`
	Context map[string]interface{} `json:"context,omitempty"`
	// Optional single-turn overrides
	ToolCallExposure *agent.ToolCallExposure `json:"toolCallExposure,omitempty"`
	AutoSummarize    *bool                   `json:"autoSummarize,omitempty"`
	// AutoSelectTools enables tool-bundle auto selection for this turn when tools are not explicitly set.
	AutoSelectTools *bool    `json:"autoSelectTools,omitempty"`
	DisableChains   bool     `json:"disableChains,omitempty"`
	AllowedChains   []string `json:"allowedChains"`
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
	_, _, _, err := s.resolveAgentIDForTurn(ctx, conversationID, req.Agent, req.Content)
	return err
}

// defaultLocation returns supplied if not empty (preserving explicit agent location).
func defaultLocation(loc string) string { return strings.TrimSpace(loc) }

const defaultMaxQueuedTurnsPerConversation = 20

// Post accepts a user message and triggers asynchronous processing via manager.
// Returns generated message ID that can be used to track status.
func (s *Service) Post(ctx context.Context, conversationID string, req PostRequest) (string, error) {
	if conversationID == "" {
		return "", fmt.Errorf("conversationID is required")
	}
	if s == nil || s.convClient == nil {
		return "", fmt.Errorf("chat service not configured: conversation client is nil")
	}
	msgID := uuid.New().String()
	debugf("Post start conversation_id=%q message_id=%q agent=%q model=%q tools=%d content_len=%d content_head=%q content_tail=%q", strings.TrimSpace(conversationID), msgID, strings.TrimSpace(req.Agent), strings.TrimSpace(req.Model), len(req.Tools), len(req.Content), headString(req.Content, 512), tailString(req.Content, 512))
	queued := queuedRequest{
		Agent:            defaultLocation(req.Agent),
		Model:            req.Model,
		Tools:            append([]string(nil), req.Tools...),
		Context:          req.Context,
		EffectiveUserID:  strings.TrimSpace(authctx.EffectiveUserID(ctx)),
		ToolCallExposure: req.ToolCallExposure,
		AutoSummarize:    req.AutoSummarize,
		AutoSelectTools:  req.AutoSelectTools,
		DisableChains:    req.DisableChains,
		AllowedChains:    append([]string(nil), req.AllowedChains...),
		Attachments:      append([]UploadedAttachment(nil), req.Attachments...),
	}
	// Capture current auth tokens (when present) so queued execution can reuse them
	// without requiring a refresh (refresh tokens may be absent in BFF flows).
	qa := &queuedAuth{}
	if tok := authctx.TokensFromContext(ctx); tok != nil {
		qa.AccessToken = strings.TrimSpace(tok.AccessToken)
		qa.IDToken = strings.TrimSpace(tok.IDToken)
		qa.Expiry = tok.Expiry
	} else {
		qa.AccessToken = strings.TrimSpace(authctx.Bearer(ctx))
		qa.IDToken = strings.TrimSpace(authctx.IDToken(ctx))
	}
	if qa.AccessToken != "" || qa.IDToken != "" {
		queued.Auth = qa
	}

	// Apply user preferences for default agent/model when not provided and when available
	if s.users != nil {
		// Resolve username from auth context (we store username in Subject for cookie sessions)
		uname := strings.TrimSpace(authctx.EffectiveUserID(ctx))
		if uname != "" {
			if u, err := s.users.FindByUsername(ctx, uname); err == nil && u != nil {
				if strings.TrimSpace(queued.Agent) == "" && u.DefaultAgentRef != nil && strings.TrimSpace(*u.DefaultAgentRef) != "" {
					queued.Agent = strings.TrimSpace(*u.DefaultAgentRef)
				}
				if strings.TrimSpace(queued.Model) == "" && u.DefaultModelRef != nil && strings.TrimSpace(*u.DefaultModelRef) != "" {
					queued.Model = strings.TrimSpace(*u.DefaultModelRef)
				}
			}
		}
	}

	if err := s.ensureQueueHasCapacity(ctx, conversationID); err != nil {
		errorf("Post queue capacity error conversation_id=%q message_id=%q err=%v", strings.TrimSpace(conversationID), msgID, err)
		return "", err
	}
	if err := s.persistQueuedTurn(ctx, conversationID, msgID, queued, req.Content); err != nil {
		errorf("Post persist queued turn error conversation_id=%q message_id=%q err=%v", strings.TrimSpace(conversationID), msgID, err)
		return "", err
	}
	if blocked, _, err := s.isConversationBlocked(ctx, conversationID); err == nil && !blocked {
		debugf("Post execute queued turn async conversation_id=%q message_id=%q", strings.TrimSpace(conversationID), msgID)
		go func() { _ = s.executeQueuedTurn(context.Background(), conversationID, msgID) }()
	} else {
		if err != nil {
			errorf("Post queue trigger due to block check error conversation_id=%q message_id=%q err=%v", strings.TrimSpace(conversationID), msgID, err)
		} else {
			debugf("Post queue trigger due to blocked conversation conversation_id=%q message_id=%q", strings.TrimSpace(conversationID), msgID)
		}
		s.triggerQueue(conversationID)
	}

	debugf("Post accepted conversation_id=%q message_id=%q", strings.TrimSpace(conversationID), msgID)
	return msgID, nil
}

type queuedRequest struct {
	Auth             *queuedAuth             `json:"auth,omitempty"`
	Agent            string                  `json:"agent,omitempty"`
	Model            string                  `json:"model,omitempty"`
	Tools            []string                `json:"tools,omitempty"`
	Context          map[string]interface{}  `json:"context,omitempty"`
	EffectiveUserID  string                  `json:"effectiveUserId,omitempty"`
	ToolCallExposure *agent.ToolCallExposure `json:"toolCallExposure,omitempty"`
	AutoSummarize    *bool                   `json:"autoSummarize,omitempty"`
	AutoSelectTools  *bool                   `json:"autoSelectTools,omitempty"`
	DisableChains    bool                    `json:"disableChains,omitempty"`
	AllowedChains    []string                `json:"allowedChains,omitempty"`
	Attachments      []UploadedAttachment    `json:"attachments,omitempty"`
}

type queuedAuth struct {
	AccessToken string    `json:"accessToken,omitempty"`
	IDToken     string    `json:"idToken,omitempty"`
	Expiry      time.Time `json:"expiry,omitempty"`
}

const queuedRequestTagPrefix = "agently:queued_request:"

func (s *Service) ensureQueueHasCapacity(ctx context.Context, conversationID string) error {
	if s == nil || s.dao == nil {
		return fmt.Errorf("chat service not configured: datly service is nil")
	}
	if strings.TrimSpace(conversationID) == "" {
		return fmt.Errorf("conversationID is required")
	}
	debugf("ensureQueueHasCapacity start conversation_id=%q", strings.TrimSpace(conversationID))
	in := &turnread.QueuedCountInput{
		ConversationID: conversationID,
		Has:            &turnread.QueuedCountInputHas{ConversationID: true},
	}
	out := &turnread.QueuedCountOutput{}
	_, err := s.dao.Operate(ctx,
		datly.WithPath(contract.NewPath("GET", turnread.QueuedCountPathURI)),
		datly.WithInput(in),
		datly.WithOutput(out),
	)
	if err != nil {
		errorf("ensureQueueHasCapacity read error conversation_id=%q err=%v", strings.TrimSpace(conversationID), err)
		return err
	}
	queued := 0
	if len(out.Data) > 0 && out.Data[0] != nil {
		queued = out.Data[0].QueuedCount
	}
	if queued >= defaultMaxQueuedTurnsPerConversation {
		debugf("ensureQueueHasCapacity full conversation_id=%q queued=%d max=%d", strings.TrimSpace(conversationID), queued, defaultMaxQueuedTurnsPerConversation)
		return fmt.Errorf("queue is full: %d queued turns (max %d)", queued, defaultMaxQueuedTurnsPerConversation)
	}
	debugf("ensureQueueHasCapacity ok conversation_id=%q queued=%d", strings.TrimSpace(conversationID), queued)
	return nil
}

func (s *Service) persistQueuedTurn(ctx context.Context, conversationID, turnID string, queued queuedRequest, content string) error {
	queueSeq := time.Now().UnixNano()
	debugf("persistQueuedTurn start conversation_id=%q turn_id=%q queue_seq=%d", strings.TrimSpace(conversationID), strings.TrimSpace(turnID), queueSeq)
	turn := apiconv.NewTurn()
	turn.SetId(turnID)
	turn.SetConversationID(conversationID)
	turn.SetQueueSeq(queueSeq)
	turn.SetStatus("queued")
	turn.SetStartedByMessageID(turnID)
	if agentRef := strings.TrimSpace(queued.Agent); agentRef != "" && !isAutoAgentRef(agentRef) {
		turn.SetAgentIDUsed(agentRef)
	}
	if strings.TrimSpace(queued.Model) != "" {
		turn.SetModelOverride(strings.TrimSpace(queued.Model))
	}
	if err := s.convClient.PatchTurn(ctx, turn); err != nil {
		errorf("persistQueuedTurn patch turn error conversation_id=%q turn_id=%q err=%v", strings.TrimSpace(conversationID), strings.TrimSpace(turnID), err)
		return err
	}
	debugf("persistQueuedTurn patched turn conversation_id=%q turn_id=%q", strings.TrimSpace(conversationID), strings.TrimSpace(turnID))

	raw, err := json.Marshal(queued)
	if err != nil {
		errorf("persistQueuedTurn marshal queued error conversation_id=%q turn_id=%q err=%v", strings.TrimSpace(conversationID), strings.TrimSpace(turnID), err)
		return err
	}

	msg := apiconv.NewMessage()
	msg.SetId(turnID)
	msg.SetConversationID(conversationID)
	msg.SetTurnID(turnID)
	msg.SetRole("user")
	msg.SetType("text")
	msg.SetContent(content)
	msg.SetTags(queuedRequestTagPrefix + string(raw))
	msg.SetStatus("pending")
	if uid := strings.TrimSpace(authctx.EffectiveUserID(ctx)); uid != "" {
		msg.SetCreatedByUserID(uid)
	}
	if err := s.convClient.PatchMessage(ctx, msg); err != nil {
		errorf("persistQueuedTurn patch message error conversation_id=%q turn_id=%q err=%v", strings.TrimSpace(conversationID), strings.TrimSpace(turnID), err)
		return err
	}
	debugf("persistQueuedTurn patched message conversation_id=%q turn_id=%q", strings.TrimSpace(conversationID), strings.TrimSpace(turnID))
	return nil
}

func (s *Service) triggerQueue(conversationID string) {
	if strings.TrimSpace(conversationID) == "" {
		return
	}
	debugf("triggerQueue conversation_id=%q", strings.TrimSpace(conversationID))
	s.queueMu.Lock()
	if s.queueByID == nil {
		s.queueByID = map[string]*conversationQueue{}
	}
	q := s.queueByID[conversationID]
	if q == nil {
		q = &conversationQueue{notify: make(chan struct{}, 1)}
		s.queueByID[conversationID] = q
		go s.runQueue(conversationID, q.notify)
	}
	ch := q.notify
	s.queueMu.Unlock()

	select {
	case ch <- struct{}{}:
	default:
	}
}

func (s *Service) runQueue(conversationID string, notify <-chan struct{}) {
	for range notify {
		debugf("runQueue notified conversation_id=%q", strings.TrimSpace(conversationID))
		for {
			if s.mgr == nil || s.dao == nil {
				debugf("runQueue stop: missing manager or dao conversation_id=%q", strings.TrimSpace(conversationID))
				return
			}

			blocked, reason, err := s.isConversationBlocked(context.Background(), conversationID)
			if err != nil {
				errorf("runQueue block check error conversation_id=%q err=%v", strings.TrimSpace(conversationID), err)
				time.Sleep(100 * time.Millisecond)
				continue
			}
			if blocked {
				debugf("runQueue blocked conversation_id=%q reason=%q", strings.TrimSpace(conversationID), reason)
				// If the conversation is waiting for user input, do not auto-retry. A user action
				// (elicitation resolution / new message) should re-trigger the queue.
				if reason == "waiting_for_user" {
					break
				}
				time.Sleep(100 * time.Millisecond)
				continue
			}

			nextID, err := s.nextQueuedTurnID(context.Background(), conversationID)
			if err != nil {
				errorf("runQueue next queued error conversation_id=%q err=%v", strings.TrimSpace(conversationID), err)
				time.Sleep(100 * time.Millisecond)
				continue
			}
			if nextID == "" {
				debugf("runQueue no queued turn conversation_id=%q", strings.TrimSpace(conversationID))
				break
			}
			debugf("runQueue execute queued turn conversation_id=%q turn_id=%q", strings.TrimSpace(conversationID), strings.TrimSpace(nextID))
			_ = s.executeQueuedTurn(context.Background(), conversationID, nextID)
		}
	}
}

func (s *Service) isConversationBlocked(ctx context.Context, conversationID string) (bool, string, error) {
	in := &turnread.ActiveTurnInput{ConversationID: conversationID, Has: &turnread.ActiveTurnInputHas{ConversationID: true}}
	svc, err := s.activeTurnService()
	if err != nil {
		return false, "", err
	}
	out, err := svc.Run(ctx, in)
	if err != nil {
		return false, "", err
	}
	if len(out.Data) == 0 || out.Data[0] == nil {
		return false, "", nil
	}
	return true, strings.ToLower(strings.TrimSpace(out.Data[0].Status)), nil
}

func (s *Service) nextQueuedTurnID(ctx context.Context, conversationID string) (string, error) {
	in := &turnread.NextQueuedInput{ConversationID: conversationID, Has: &turnread.NextQueuedInputHas{ConversationID: true}}
	svc, err := s.nextQueuedService()
	if err != nil {
		return "", err
	}
	out, err := svc.Run(ctx, in)
	if err != nil {
		return "", err
	}
	if len(out.Data) == 0 || out.Data[0] == nil {
		return "", nil
	}
	return out.Data[0].Id, nil
}

func (s *Service) activeTurnService() (*component.Service[turnread.ActiveTurnInput, turnread.ActiveTurnOutput], error) {
	if s.activeTurnSvc != nil {
		return s.activeTurnSvc, nil
	}
	if s.dao == nil {
		return nil, fmt.Errorf("datly service is not configured")
	}
	s.activeTurnSvc = component.NewWithInjector[turnread.ActiveTurnInput, turnread.ActiveTurnOutput](
		xhttp.NewRoute("GET", turnread.ActiveTurnPathURI),
		s.componentInjector,
	)
	return s.activeTurnSvc, nil
}

func (s *Service) nextQueuedService() (*component.Service[turnread.NextQueuedInput, turnread.NextQueuedOutput], error) {
	if s.nextQueuedSvc != nil {
		return s.nextQueuedSvc, nil
	}
	if s.dao == nil {
		return nil, fmt.Errorf("datly service is not configured")
	}
	s.nextQueuedSvc = component.NewWithInjector[turnread.NextQueuedInput, turnread.NextQueuedOutput](
		xhttp.NewRoute("GET", turnread.NextQueuedPathURI),
		s.componentInjector,
	)
	return s.nextQueuedSvc, nil
}

func (s *Service) componentInjector(ctx context.Context, route xhttp.Route) (state.Injector, error) {
	comp, err := s.dao.Component(ctx, route.Method+":"+route.URL)
	if err != nil {
		return nil, err
	}
	sess := s.dao.NewComponentSession(comp)
	handlerSess, err := s.dao.HandlerSession(ctx, comp, sess)
	if err != nil {
		return nil, err
	}
	return handlerSess.Stater(), nil
}

func (s *Service) executeQueuedTurn(parent context.Context, conversationID, turnID string) error {
	debugf("executeQueuedTurn start conversation_id=%q turn_id=%q", strings.TrimSpace(conversationID), strings.TrimSpace(turnID))
	msg, err := s.convClient.GetMessage(parent, turnID)
	if err != nil {
		errorf("executeQueuedTurn get message error conversation_id=%q turn_id=%q err=%v", strings.TrimSpace(conversationID), strings.TrimSpace(turnID), err)
		return s.persistTurnFailure(parent, turnID, err)
	}
	if msg == nil {
		debugf("executeQueuedTurn message missing conversation_id=%q turn_id=%q", strings.TrimSpace(conversationID), strings.TrimSpace(turnID))
		return s.persistTurnFailure(parent, turnID, fmt.Errorf("queued turn message not found: %s", turnID))
	}

	var meta queuedRequest
	if payload, ok := queuedRequestPayload(msg); ok {
		if uerr := json.Unmarshal([]byte(payload), &meta); uerr != nil {
			errorf("executeQueuedTurn decode payload error conversation_id=%q turn_id=%q err=%v", strings.TrimSpace(conversationID), strings.TrimSpace(turnID), uerr)
			return s.persistTurnFailure(parent, turnID, fmt.Errorf("decode queued request: %w", uerr))
		}
	}
	debugf("executeQueuedTurn loaded payload conversation_id=%q turn_id=%q", strings.TrimSpace(conversationID), strings.TrimSpace(turnID))

	query := ""
	if msg.Content != nil {
		query = *msg.Content
	}
	debugf("executeQueuedTurn user_prompt conversation_id=%q turn_id=%q prompt_len=%d prompt_head=%q prompt_tail=%q", strings.TrimSpace(conversationID), strings.TrimSpace(turnID), len(query), headString(query, 512), tailString(query, 512))
	agentID, autoSelected, _, err := s.resolveAgentIDForTurn(parent, conversationID, meta.Agent, query)
	if err != nil {
		errorf("executeQueuedTurn resolve agent error conversation_id=%q turn_id=%q err=%v", strings.TrimSpace(conversationID), strings.TrimSpace(turnID), err)
		return s.persistTurnFailure(parent, turnID, err)
	}
	debugf("executeQueuedTurn resolved agent conversation_id=%q turn_id=%q agent_id=%q auto_selected=%v", strings.TrimSpace(conversationID), strings.TrimSpace(turnID), strings.TrimSpace(agentID), autoSelected)
	if autoSelected && strings.TrimSpace(agentID) != "" && strings.TrimSpace(agentID) != "agent_selector" {
		upd := apiconv.NewTurn()
		upd.SetId(turnID)
		upd.SetConversationID(conversationID)
		upd.SetAgentIDUsed(strings.TrimSpace(agentID))
		if perr := s.convClient.PatchTurn(parent, upd); perr != nil {
			errorf("executeQueuedTurn patch agent error conversation_id=%q turn_id=%q err=%v", strings.TrimSpace(conversationID), strings.TrimSpace(turnID), perr)
			return s.persistTurnFailure(parent, turnID, perr)
		}
	}
	var input *agentpkg.QueryInput
	if strings.TrimSpace(agentID) == "agent_selector" {
		capPrompt := prompts.CapabilityPrompt()
		if s != nil && s.defaults != nil && strings.TrimSpace(s.defaults.CapabilityPrompt) != "" {
			capPrompt = strings.TrimSpace(s.defaults.CapabilityPrompt)
		}
		capAgent := &agent.Agent{
			Identity: agent.Identity{ID: "agent_selector", Name: "Agent Selector"},
			Prompt:   &promptpkg.Prompt{Text: "{{.Task.Prompt}}", Engine: "go"},
			SystemPrompt: &promptpkg.Prompt{
				Text:   capPrompt,
				Engine: "go",
			},
			Persona: &promptpkg.Persona{Role: "assistant", Actor: "Capability"},
		}
		autoTools := false
		input = &agentpkg.QueryInput{
			ConversationID:   conversationID,
			Query:            query,
			Agent:            capAgent,
			ModelOverride:    meta.Model,
			ToolsAllowed:     []string{"llm/agents:list"},
			AutoSelectTools:  &autoTools,
			Context:          meta.Context,
			MessageID:        turnID,
			ToolCallExposure: meta.ToolCallExposure,
			AutoSummarize:    meta.AutoSummarize,
			DisableChains:    true,
		}
	} else {
		input = &agentpkg.QueryInput{
			ConversationID:   conversationID,
			Query:            query,
			AgentID:          strings.TrimSpace(agentID),
			ModelOverride:    meta.Model,
			ToolsAllowed:     append([]string(nil), meta.Tools...),
			AutoSelectTools:  meta.AutoSelectTools,
			Context:          meta.Context,
			MessageID:        turnID,
			ToolCallExposure: meta.ToolCallExposure,
			AutoSummarize:    meta.AutoSummarize,
			DisableChains:    meta.DisableChains,
			AllowedChains:    append([]string(nil), meta.AllowedChains...),
		}
	}
	debugf("executeQueuedTurn built input conversation_id=%q turn_id=%q agent_selector=%v", strings.TrimSpace(conversationID), strings.TrimSpace(turnID), strings.TrimSpace(agentID) == "agent_selector")

	// Detach from caller cancellation but preserve identity for per-user jars.
	base := context.Background()
	// Restore captured auth (if available) from queued request metadata.
	// This avoids relying on refresh flows that may be unavailable in BFF mode.
	if meta.Auth != nil {
		at := strings.TrimSpace(meta.Auth.AccessToken)
		idt := strings.TrimSpace(meta.Auth.IDToken)
		if at != "" || idt != "" {
			base = authctx.WithTokens(base, &scyauth.Token{Token: oauth2.Token{
				AccessToken: at,
				TokenType:   "Bearer",
				Expiry:      meta.Auth.Expiry,
			}, IDToken: idt})
			if at != "" {
				base = authctx.WithBearer(base, at)
			}
			if idt != "" {
				base = authctx.WithIDToken(base, idt)
			}
		}
	}
	// Preserve auth tokens from the triggering context so queued execution can
	// reuse them for MCP/tool calls (per-server token selection happens at call time).
	if tok := authctx.TokensFromContext(parent); tok != nil {
		base = authctx.WithTokens(base, tok)
		if strings.TrimSpace(tok.AccessToken) != "" {
			base = authctx.WithBearer(base, strings.TrimSpace(tok.AccessToken))
		}
		if strings.TrimSpace(tok.IDToken) != "" {
			base = authctx.WithIDToken(base, strings.TrimSpace(tok.IDToken))
		}
	} else {
		if v := strings.TrimSpace(authctx.Bearer(parent)); v != "" {
			base = authctx.WithBearer(base, v)
		}
		if v := strings.TrimSpace(authctx.IDToken(parent)); v != "" {
			base = authctx.WithIDToken(base, v)
		}
	}
	effectiveUserID := strings.TrimSpace(meta.EffectiveUserID)
	if effectiveUserID == "" && msg.CreatedByUserId != nil {
		effectiveUserID = strings.TrimSpace(*msg.CreatedByUserId)
	}
	if effectiveUserID != "" {
		base = authctx.WithUserInfo(base, &authctx.UserInfo{Subject: effectiveUserID})
	}

	runCtx, cancel := context.WithCancel(base)
	defer cancel()
	if s.reg != nil {
		s.reg.Register(conversationID, turnID, cancel)
		defer s.reg.Complete(conversationID, turnID, cancel)
	}

	// Best-effort: attach an access token for MCP/tool auth when BFF/OAuth is configured.
	runCtx = s.ensureBearerForTools(runCtx, effectiveUserID)

	// Apply policy and conversation ID.
	runCtx = memory.WithConversationID(runCtx, conversationID)
	if s.toolPolicy != nil {
		runCtx = tool.WithPolicy(runCtx, s.toolPolicy)
	} else {
		runCtx = tool.WithPolicy(runCtx, &tool.Policy{Mode: tool.ModeAuto})
	}
	if pol := tool.FromContext(parent); pol != nil {
		runCtx = tool.WithPolicy(runCtx, pol)
	}

	// Populate userId for attribution when missing.
	if strings.TrimSpace(input.UserId) == "" {
		if ui := authctx.User(runCtx); ui != nil {
			user := strings.TrimSpace(ui.Subject)
			if user == "" {
				user = strings.TrimSpace(ui.Email)
			}
			if user != "" {
				input.UserId = user
			}
		}
		if strings.TrimSpace(input.UserId) == "" {
			input.UserId = "anonymous"
		}
	}

	// Convert staged uploads into attachments (read + cleanup).
	// Queueing assumes staged artifacts remain accessible until this turn runs.
	if err := s.enrichAttachmentIfNeeded(PostRequest{Attachments: meta.Attachments}, runCtx, input); err != nil {
		errorf("executeQueuedTurn enrich attachments error conversation_id=%q turn_id=%q err=%v", strings.TrimSpace(conversationID), strings.TrimSpace(turnID), err)
		return err
	}

	debugf("executeQueuedTurn manager accept start conversation_id=%q turn_id=%q", strings.TrimSpace(conversationID), strings.TrimSpace(turnID))
	out, err := s.mgr.Accept(runCtx, input)
	if err != nil {
		errorf("executeQueuedTurn manager accept error conversation_id=%q turn_id=%q err=%v", strings.TrimSpace(conversationID), strings.TrimSpace(turnID), err)
		return s.persistTurnFailure(runCtx, turnID, err)
	}
	if out != nil && out.Elicitation != nil && !out.Elicitation.IsEmpty() {
		upd := apiconv.NewTurn()
		upd.SetId(turnID)
		upd.SetStatus("waiting_for_user")
		debugf("executeQueuedTurn waiting_for_user conversation_id=%q turn_id=%q", strings.TrimSpace(conversationID), strings.TrimSpace(turnID))
		return s.convClient.PatchTurn(context.Background(), upd)
	}
	debugf("executeQueuedTurn completed conversation_id=%q turn_id=%q", strings.TrimSpace(conversationID), strings.TrimSpace(turnID))
	return nil
}

func queuedRequestPayload(msg *apiconv.Message) (string, bool) {
	if msg == nil {
		return "", false
	}
	if msg.Tags != nil {
		s := strings.TrimSpace(*msg.Tags)
		if strings.HasPrefix(s, queuedRequestTagPrefix) {
			s = strings.TrimSpace(strings.TrimPrefix(s, queuedRequestTagPrefix))
			if s != "" {
				return s, true
			}
		}
	}
	// Backward compatibility: older queued messages stored metadata in raw_content.
	if msg.RawContent != nil {
		s := strings.TrimSpace(*msg.RawContent)
		if s != "" && (strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[")) {
			return s, true
		}
	}
	return "", false
}

func (s *Service) persistTurnFailure(ctx context.Context, turnID string, err error) error {
	if err == nil || s == nil || s.convClient == nil {
		return err
	}
	upd := apiconv.NewTurn()
	upd.SetId(turnID)
	if errors.Is(err, context.Canceled) {
		upd.SetStatus("canceled")
		warnf("persistTurnFailure canceled turn_id=%q err=%v", strings.TrimSpace(turnID), err)
		return s.convClient.PatchTurn(context.Background(), upd)
	}
	upd.SetStatus("failed")
	upd.SetErrorMessage(err.Error())
	errorf("persistTurnFailure failed turn_id=%q err=%v", strings.TrimSpace(turnID), err)
	patchCtx := ctx
	if errors.Is(ctx.Err(), context.Canceled) {
		patchCtx = context.Background()
	}
	if pErr := s.convClient.PatchTurn(patchCtx, upd); pErr != nil {
		errorf("persistTurnFailure patch error turn_id=%q err=%v", strings.TrimSpace(turnID), pErr)
		return fmt.Errorf("%w: %v", err, pErr)
	}
	return err
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

// CancelQueuedTurn cancels a queued (not yet running) turn and marks its
// starting user message as canceled as well.
func (s *Service) CancelQueuedTurn(ctx context.Context, conversationID, turnID string) error {
	if s == nil || s.dao == nil || s.convClient == nil {
		return fmt.Errorf("chat service not configured")
	}
	if strings.TrimSpace(conversationID) == "" || strings.TrimSpace(turnID) == "" {
		return fmt.Errorf("conversationID and turnID are required")
	}

	in := &turnread.TurnByIDInput{
		ID:             turnID,
		ConversationID: conversationID,
		Has:            &turnread.TurnByIDInputHas{ID: true, ConversationID: true},
	}
	out := &turnread.TurnByIDOutput{}
	_, err := s.dao.Operate(ctx,
		datly.WithPath(contract.NewPath("GET", turnread.TurnByIDPathURI)),
		datly.WithInput(in),
		datly.WithOutput(out),
	)
	if err != nil {
		return err
	}
	if len(out.Data) == 0 || out.Data[0] == nil {
		return fmt.Errorf("turn not found")
	}

	st := strings.ToLower(strings.TrimSpace(out.Data[0].Status))
	if st != "queued" {
		return fmt.Errorf("cannot cancel turn in status %q", out.Data[0].Status)
	}

	turnUpd := apiconv.NewTurn()
	turnUpd.SetId(turnID)
	turnUpd.SetStatus("canceled")
	if err := s.convClient.PatchTurn(ctx, turnUpd); err != nil {
		return err
	}

	// Best-effort update of the starting message status; message schema uses 'cancel'.
	msgUpd := apiconv.NewMessage()
	msgUpd.SetId(turnID)
	msgUpd.SetConversationID(conversationID)
	msgUpd.SetStatus("cancel")
	_ = s.convClient.PatchMessage(ctx, msgUpd)
	return nil
}

func (s *Service) MoveQueuedTurn(ctx context.Context, conversationID, turnID, direction string) error {
	if s == nil || s.dao == nil || s.convClient == nil {
		return fmt.Errorf("chat service not configured")
	}
	if strings.TrimSpace(conversationID) == "" || strings.TrimSpace(turnID) == "" {
		return fmt.Errorf("conversationID and turnID are required")
	}

	dir := strings.ToLower(strings.TrimSpace(direction))
	if dir != "up" && dir != "down" {
		return fmt.Errorf("unsupported direction %q", direction)
	}

	in := &turnread.QueuedListInput{
		ConversationID: conversationID,
		Has:            &turnread.QueuedListInputHas{ConversationID: true},
	}
	out := &turnread.QueuedListOutput{}
	_, err := s.dao.Operate(ctx,
		datly.WithPath(contract.NewPath("GET", turnread.QueuedListPathURI)),
		datly.WithInput(in),
		datly.WithOutput(out),
	)
	if err != nil {
		return err
	}
	if len(out.Data) == 0 {
		return fmt.Errorf("no queued turns")
	}

	currentIndex := -1
	for i, v := range out.Data {
		if v == nil {
			continue
		}
		if strings.TrimSpace(v.ID) == strings.TrimSpace(turnID) {
			currentIndex = i
			break
		}
	}
	if currentIndex == -1 {
		return fmt.Errorf("turn not found or not queued")
	}

	targetIndex := currentIndex
	if dir == "up" {
		targetIndex = currentIndex - 1
	} else {
		targetIndex = currentIndex + 1
	}
	if targetIndex < 0 || targetIndex >= len(out.Data) || out.Data[targetIndex] == nil {
		return nil
	}

	a := out.Data[currentIndex]
	b := out.Data[targetIndex]
	queueA := a.QueueSeq
	queueB := b.QueueSeq

	// Ensure both have a value to swap. When missing, assign stable values.
	if queueA == nil || queueB == nil {
		base := time.Now().UnixNano()
		if queueA == nil {
			v := base
			queueA = &v
		}
		if queueB == nil {
			v := base + 1
			queueB = &v
		}
	}

	turnA := apiconv.NewTurn()
	turnA.SetId(a.ID)
	turnA.SetConversationID(conversationID)
	turnA.SetQueueSeq(*queueB)
	if err := s.convClient.PatchTurn(ctx, turnA); err != nil {
		return err
	}

	turnB := apiconv.NewTurn()
	turnB.SetId(b.ID)
	turnB.SetConversationID(conversationID)
	turnB.SetQueueSeq(*queueA)
	if err := s.convClient.PatchTurn(ctx, turnB); err != nil {
		return err
	}

	return nil
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
	Shareable  bool   `json:"shareable"`
}

// PatchConversationRequest mirrors HTTP payload for PATCH /conversations/{id}.
type PatchConversationRequest struct {
	Visibility string `json:"visibility"`
	Shareable  *bool  `json:"shareable"`
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
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	Summary    *string   `json:"summary"`
	Visibility string    `json:"visibility,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
	Agent      string    `json:"agent,omitempty"`
	Model      string    `json:"model,omitempty"`
	Tools      []string  `json:"tools,omitempty"`
	Stage      string    `json:"stage,omitempty"`
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
	vis := strings.ToLower(strings.TrimSpace(in.Visibility))
	if vis != convw.VisibilityPublic && vis != convw.VisibilityPrivate {
		vis = convw.VisibilityPrivate
	}
	cw.SetVisibility(vis)
	if in.Shareable {
		cw.SetShareable(true)
	}
	if s := strings.TrimSpace(in.Agent); s != "" {
		cw.SetAgentId(s)
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

// PatchConversation updates mutable fields (currently visibility) on an existing conversation.
func (s *Service) PatchConversation(ctx context.Context, id string, in PatchConversationRequest) error {
	convID := strings.TrimSpace(id)
	if convID == "" {
		return fmt.Errorf("conversation id is required")
	}
	vis := strings.ToLower(strings.TrimSpace(in.Visibility))
	if vis != "" && vis != convw.VisibilityPublic && vis != convw.VisibilityPrivate {
		return fmt.Errorf("invalid visibility")
	}
	if vis == "" && in.Shareable == nil {
		return fmt.Errorf("no fields to update")
	}
	cv, err := s.convClient.GetConversation(ctx, convID)
	if err != nil {
		return err
	}
	if cv == nil {
		return fmt.Errorf("conversation not found")
	}
	if ui := authctx.User(ctx); ui != nil {
		want := strings.TrimSpace(ui.Subject)
		if want == "" {
			want = strings.TrimSpace(ui.Email)
		}
		if want == "" {
			return fmt.Errorf("missing user identity")
		}
		if cv.CreatedByUserId != nil && strings.TrimSpace(*cv.CreatedByUserId) != want {
			return fmt.Errorf("forbidden")
		}
	} else if cv.CreatedByUserId != nil && strings.TrimSpace(*cv.CreatedByUserId) != "" {
		return fmt.Errorf("forbidden")
	}
	cw := &convw.Conversation{Has: &convw.ConversationHas{}}
	cw.SetId(convID)
	if vis != "" {
		cw.SetVisibility(vis)
	}
	if in.Shareable != nil {
		cw.SetShareable(*in.Shareable)
	}
	cw.SetUpdatedAt(time.Now().UTC())
	if err := s.convClient.PatchConversations(ctx, (*apiconv.MutableConversation)(cw)); err != nil {
		return fmt.Errorf("failed to patch conversation: %w", err)
	}
	return nil
}

// GetConversation returns id + title by conversation id.
func (s *Service) GetConversation(ctx context.Context, id string) (*ConversationSummary, error) {
	// Include transcript so Stage can be computed accurately.
	cv, err := s.convClient.GetConversation(ctx, id, func(input *apiconv.Input) {
		if input == nil {
			return
		}
		input.IncludeTranscript = true
		if input.Has == nil {
			input.Has = &agconv.ConversationInputHas{}
		}
		input.Has.IncludeTranscript = true
	})
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
	var tools []string
	if cv.Metadata != nil && strings.TrimSpace(*cv.Metadata) != "" {
		var meta agentpkg.ConversationMetadata
		_ = json.Unmarshal([]byte(*cv.Metadata), &meta)
		if len(meta.Tools) > 0 {
			tools = append([]string(nil), meta.Tools...)
		}
	}
	agentID := ""
	if cv.AgentId != nil {
		agentID = strings.TrimSpace(*cv.AgentId)
	}
	model := ""
	if cv.DefaultModel != nil {
		model = strings.TrimSpace(*cv.DefaultModel)
	}
	vis := strings.ToLower(strings.TrimSpace(cv.Visibility))
	return &ConversationSummary{ID: id, Title: t, Summary: cv.Summary, CreatedAt: cv.CreatedAt, Visibility: vis, Agent: agentID, Model: model, Tools: tools}, nil
}

// ListConversations returns all conversation summaries.
func (s *Service) ListConversations(ctx context.Context, input *apiconv.Input) ([]ConversationSummary, error) {
	rows, err := s.convClient.GetConversations(ctx, input)
	if err != nil {
		return nil, err
	}
	// Schedule history (hasScheduleId=true) should include all conversations
	// visible to the caller, not only caller-owned ones.
	ownerOnly := true
	if input != nil && input.HasScheduleId {
		ownerOnly = false
	}
	// Resolve current user
	var userID string
	if ui := authctx.User(ctx); ui != nil {
		userID = strings.TrimSpace(ui.Subject)
		if userID == "" {
			userID = strings.TrimSpace(ui.Email)
		}
	}
	type convo struct {
		summary  ConversationSummary
		lastSeen time.Time
	}
	tmp := make([]convo, 0, len(rows))
	for _, v := range rows {
		if v == nil {
			continue
		}
		if ownerOnly {
			// Default conversation list remains owner-scoped.
			if userID == "" || v.CreatedByUserId == nil || strings.TrimSpace(*v.CreatedByUserId) != userID {
				continue
			}
		}
		t := v.Id
		if v.Title != nil && strings.TrimSpace(*v.Title) != "" {
			t = *v.Title
		}
		var tools []string
		if v.Metadata != nil && strings.TrimSpace(*v.Metadata) != "" {
			var meta agentpkg.ConversationMetadata
			_ = json.Unmarshal([]byte(*v.Metadata), &meta)
			if len(meta.Tools) > 0 {
				tools = append([]string(nil), meta.Tools...)
			}
		}
		agentID := ""
		if v.AgentId != nil {
			agentID = strings.TrimSpace(*v.AgentId)
		}
		model := ""
		if v.DefaultModel != nil {
			model = strings.TrimSpace(*v.DefaultModel)
		}
		lastSeen := v.CreatedAt
		if v.LastActivity != nil && !v.LastActivity.IsZero() {
			lastSeen = *v.LastActivity
		} else if v.UpdatedAt != nil && !v.UpdatedAt.IsZero() {
			lastSeen = *v.UpdatedAt
		}
		vis := strings.ToLower(strings.TrimSpace(v.Visibility))
		tmp = append(tmp, convo{
			summary:  ConversationSummary{ID: v.Id, Title: t, Summary: v.Summary, CreatedAt: v.CreatedAt, Visibility: vis, Agent: agentID, Model: model, Tools: tools},
			lastSeen: lastSeen,
		})
	}
	sort.SliceStable(tmp, func(i, j int) bool {
		if tmp[i].lastSeen.Equal(tmp[j].lastSeen) {
			return strings.TrimSpace(tmp[i].summary.ID) > strings.TrimSpace(tmp[j].summary.ID)
		}
		return tmp[i].lastSeen.After(tmp[j].lastSeen)
	})
	out := make([]ConversationSummary, 0, len(tmp))
	for _, c := range tmp {
		out = append(out, c.summary)
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
	if err := s.elicitation.Resolve(ctx, elicitationMsg.ConversationId, *elicitationMsg.ElicitationId, action, payload, ""); err != nil {
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
	body := *p.InlineBody
	return body, ctype, nil
}

// looksLikeJSON performs a quick check if the payload appears to be JSON.
func looksLikeJSON(b []byte) bool {
	s := strings.TrimSpace(string(b))
	if s == "" {
		return false
	}
	c := s[0]
	return c == '{' || c == '['
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
	status = shared.NormalizeMessageStatus(status)
	upd := apiconv.NewMessage()
	upd.SetId(messageID)
	upd.SetStatus(status)
	return s.convClient.PatchMessage(ctx, upd)
}

// SetLastAssistentMessageStatus updates the latest turn in a conversation.
func (s *Service) SetLastAssistentMessageStatus(ctx context.Context, conversationID, status string) error {
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
	if err != nil {
		return fmt.Errorf("failed to get conversation: %v %w", conversationID, err)
	}

	if conv.Status != nil && (*conv.Status == "compacting" || *conv.Status == "compacted") {
		return nil
	}

	status := "compacting"
	err = s.SetConversationStatus(ctx, conversationID, status)
	if err != nil {
		return err
	}

	ctx = memory.WithTurnMeta(ctx, memory.TurnMeta{ConversationID: conversationID, TurnID: *conv.LastTurnId, ParentMessageID: *conv.LastTurnId})
	transcript := conv.GetTranscript()
	// Try LLM-generated summary via manager first
	summaryMessageID, err := s.compactGenerateSummaryLLM(ctx, conv)
	if err != nil {
		return fmt.Errorf("failed to  compact summary message: %v %w", conversationID, err)
	}
	s.compactMessagePriorMessageID(ctx, transcript, summaryMessageID)

	status = "compacted"
	err = s.SetConversationStatus(ctx, conversationID, status)
	if err != nil {
		return err
	}

	return nil
}

// Prune removes low-value tool outputs and older messages using an LLM-guided selection.
func (s *Service) Prune(ctx context.Context, conversationID string) error {
	if s == nil || s.convClient == nil || strings.TrimSpace(conversationID) == "" {
		return nil
	}
	if s.core == nil || s.agentFinder == nil {
		return fmt.Errorf("missing core or agent finder")
	}

	conv, err := s.convClient.GetConversation(ctx, conversationID, apiconv.WithIncludeToolCall(true))
	if err != nil {
		return fmt.Errorf("failed to get conversation: %v %w", conversationID, err)
	}
	if conv.Status != nil && (*conv.Status == "pruning" || *conv.Status == "pruned") {
		return nil
	}

	if err := s.SetConversationStatus(ctx, conversationID, "pruning"); err != nil {
		return err
	}

	turnID := ""
	if conv.LastTurnId != nil {
		turnID = *conv.LastTurnId
	}
	ctx = memory.WithConversationID(ctx, conversationID)
	ctx = memory.WithTurnMeta(ctx, memory.TurnMeta{ConversationID: conversationID, TurnID: turnID, ParentMessageID: turnID})

	if err := s.pruneMessagesLLM(ctx, conv); err != nil {
		return fmt.Errorf("failed to prune conversation: %w", err)
	}

	if err := s.SetConversationStatus(ctx, conversationID, "pruned"); err != nil {
		return err
	}
	return nil
}

func (s *Service) SetConversationStatus(ctx context.Context, conversationID string, status string) error {
	debugf("SetConversationStatus start conversation_id=%q status=%q", strings.TrimSpace(conversationID), strings.TrimSpace(status))
	if err := s.convClient.PatchConversations(ctx, convw.NewConversationStatus(conversationID, status)); err != nil {
		errorf("SetConversationStatus error conversation_id=%q status=%q err=%v", strings.TrimSpace(conversationID), strings.TrimSpace(status), err)
		return fmt.Errorf("failed to update conversation: %w", err)
	}
	debugf("SetConversationStatus ok conversation_id=%q status=%q", strings.TrimSpace(conversationID), strings.TrimSpace(status))
	return nil
}

// insertSummaryMessage writes a single assistant summary message and returns its id.
func (s *Service) insertSummaryMessage(ctx context.Context, conversationID, summary string) (string, error) {
	turn, ok := memory.TurnMetaFromContext(ctx)
	if !ok {
		return "", errors.New("no turn meta")
	}
	msgID := uuid.NewString()
	msg, err := apiconv.AddMessage(ctx, s.convClient, &turn,
		apiconv.WithId(msgID),
		apiconv.WithConversationID(conversationID),
		apiconv.WithRole("assistant"),
		apiconv.WithType("text"),
		apiconv.WithStatus("summary"),
		apiconv.WithContent(summary),
	)
	if err != nil {
		return "", err
	}
	return msg.Id, err
}

// compactMessagePriorMessageID sets archived=1 on all prior messages except elicitation and excludeMsgID.
func (s *Service) compactMessagePriorMessageID(ctx context.Context, transcript apiconv.Transcript, excludeMsgID string) {
	for _, t := range transcript {
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
			upd.SetArchived(1)
			_ = s.convClient.PatchMessage(ctx, upd)
		}
	}
}

// compactGenerateSummaryLLM performs a one-off LLM turn to summarize the conversation and returns the assistant message id.
func (s *Service) compactGenerateSummaryLLM(ctx context.Context, conv *apiconv.Conversation) (string, error) {
	tr := conv.GetTranscript()
	if conv.AgentId == nil {
		return "", fmt.Errorf("agent id is missing in conversation: %v", conv.Id)
	}
	agentId := *conv.AgentId
	anAgent, err := s.agentFinder.Find(ctx, agentId)
	if err != nil {
		return "", fmt.Errorf("failed to  find agent: %v %w", conv.AgentId, err)
	}

	var msgs []llm.Message
	// Determine slice size from defaults
	maxN := 50
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

	// Reverse to chronological order
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}

	//compactInstruction := "Summarize key points to continue the discussion. Don't comment and analyze, just summarize. Be concise (<=6 bullets), include goals/decisions/next steps, avoid logs/quotes. Exclude the latest message."
	if s.defaults != nil && strings.TrimSpace(s.defaults.SummaryPrompt) != "" {
		compactInstruction = strings.TrimSpace(s.defaults.SummaryPrompt)
	}
	msgs = append(msgs, llm.NewUserMessage(compactInstruction))
	model := ""

	if s.defaults != nil && strings.TrimSpace(s.defaults.SummaryModel) != "" {
		model = strings.TrimSpace(s.defaults.SummaryModel)
	} else if conv.DefaultModel != nil && strings.TrimSpace(*conv.DefaultModel) != "" {
		model = strings.TrimSpace(*conv.DefaultModel)
	}

	in := &corellm.GenerateInput{ModelSelection: llm.ModelSelection{Model: model}, Message: msgs}
	var out corellm.GenerateOutput

	agentsrv.EnsureGenerateOptions(ctx, in, anAgent)
	in.Options.Mode = "compact"

	if err := s.core.Generate(ctx, in, &out); err != nil {
		return "", err
	}
	content := strings.TrimSpace(out.Content)
	if content == "" {
		return "", fmt.Errorf("empty summary")
	}
	id, ierr := s.insertSummaryMessage(ctx, conv.Id, content)
	if ierr != nil {
		return "", ierr
	}
	return id, nil
}

func (s *Service) pruneMessagesLLM(ctx context.Context, conv *apiconv.Conversation) error {
	if conv == nil {
		return fmt.Errorf("missing conversation")
	}
	if conv.AgentId == nil {
		return fmt.Errorf("agent id is missing in conversation: %v", conv.Id)
	}
	agentID := *conv.AgentId
	anAgent, err := s.agentFinder.Find(ctx, agentID)
	if err != nil {
		return fmt.Errorf("failed to find agent: %v %w", conv.AgentId, err)
	}

	msgService := msgsvc.New(s.convClient)
	listExec, err := msgService.Method("listCandidates")
	if err != nil {
		return err
	}
	var listOut msgsvc.ListCandidatesOutput
	listIn := &msgsvc.ListCandidatesInput{Limit: pruneMaxRemove, Types: []string{"tool", "assistant", "user"}}
	if err := listExec(ctx, listIn, &listOut); err != nil {
		return err
	}
	lines, ids := buildPruneCandidateLines(listOut.Candidates)
	if len(lines) == 0 {
		return nil
	}
	promptText := composePrunePrompt(lines, ids)

	model := ""
	if s.defaults != nil && strings.TrimSpace(s.defaults.SummaryModel) != "" {
		model = strings.TrimSpace(s.defaults.SummaryModel)
	} else if conv.DefaultModel != nil && strings.TrimSpace(*conv.DefaultModel) != "" {
		model = strings.TrimSpace(*conv.DefaultModel)
	}
	in := &corellm.GenerateInput{
		ModelSelection: llm.ModelSelection{Model: model},
		Message:        []llm.Message{llm.NewUserMessage(promptText)},
	}
	agentsrv.EnsureGenerateOptions(ctx, in, anAgent)
	in.Options.Mode = "compact"
	var out corellm.GenerateOutput
	if err := s.core.Generate(ctx, in, &out); err != nil {
		return err
	}
	payload := strings.TrimSpace(out.Content)
	if payload == "" {
		return fmt.Errorf("empty prune response")
	}
	jsonBody, err := extractFirstJSON(payload)
	if err != nil {
		return err
	}
	var removeIn msgsvc.RemoveInput
	if uerr := json.Unmarshal([]byte(jsonBody), &removeIn); uerr != nil {
		return fmt.Errorf("failed to parse prune response: %w", uerr)
	}
	if len(removeIn.Tuples) == 0 {
		return fmt.Errorf("prune response missing tuples")
	}
	removeExec, err := msgService.Method("remove")
	if err != nil {
		return err
	}
	var removeOut msgsvc.RemoveOutput
	if err := removeExec(ctx, &removeIn, &removeOut); err != nil {
		return err
	}
	return nil
}

func buildPruneCandidateLines(cands []msgsvc.Candidate) ([]string, []string) {
	lines := make([]string, 0, len(cands))
	ids := make([]string, 0, len(cands))
	for _, c := range cands {
		line := ""
		switch strings.ToLower(strings.TrimSpace(c.Type)) {
		case "tool":
			line = fmt.Sprintf("messageId: %s, type: tool, tool: %s, args_preview: \"%s\", size: %d bytes (~%d tokens)", c.MessageID, c.ToolName, c.Preview, c.ByteSize, c.TokenSize)
		default:
			line = fmt.Sprintf("messageId: %s, type: %s, preview: \"%s\", size: %d bytes (~%d tokens)", c.MessageID, c.Type, c.Preview, c.ByteSize, c.TokenSize)
		}
		lines = append(lines, line)
		ids = append(ids, c.MessageID)
	}
	return lines, ids
}

func composePrunePrompt(lines []string, ids []string) string {
	tpl := prompts.Prune
	tpl = strings.Replace(tpl, "{{ERROR_MESSAGE}}", "manual prune requested by user", 1)
	tpl = strings.ReplaceAll(tpl, "{{REMOVE_MIN}}", strconv.Itoa(pruneMinRemove))
	tpl = strings.ReplaceAll(tpl, "{{REMOVE_MAX}}", strconv.Itoa(pruneMaxRemove))
	var buf bytes.Buffer
	if len(ids) > 0 {
		buf.WriteString("The following message IDs are provided inside a fenced code block.\n")
		buf.WriteString("Copy them exactly in tool args; do not alter formatting.\n\n")
		buf.WriteString("```text\n")
		for _, id := range ids {
			buf.WriteString(id)
			buf.WriteByte('\n')
		}
		buf.WriteString("```\n\n")
		buf.WriteString("Candidates for removal:\n")
	}
	for _, l := range lines {
		buf.WriteString(l)
		buf.WriteByte('\n')
	}
	return tpl + "\n\n" + buf.String()
}

// extractFirstJSON scans the payload for the first complete JSON object or array.
// It tolerates leading/trailing noise and respects strings/escapes.
func extractFirstJSON(payload string) (string, error) {
	b := []byte(payload)
	inString := false
	escape := false
	depth := 0
	start := -1
	for i, c := range b {
		if inString {
			if escape {
				escape = false
				continue
			}
			if c == '\\' {
				escape = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case ' ', '\t', '\r', '\n':
			// skip whitespace outside strings
			continue
		case '"':
			inString = true
			if depth == 0 { // strings before JSON start are noise
				continue
			}
		case '{', '[':
			if depth == 0 {
				start = i
			}
			depth++
		case '}', ']':
			if depth > 0 {
				depth--
			}
			if depth == 0 && start >= 0 {
				return string(b[start : i+1]), nil
			}
		default:
			// other characters before JSON start are ignored
		}
	}
	return "", fmt.Errorf("no JSON object found in prune response")
}
