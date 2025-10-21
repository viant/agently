package agents

import (
	"context"
	"reflect"
	"strings"

	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/memory"
	agentsvc "github.com/viant/agently/genai/service/agent"
	linksvc "github.com/viant/agently/genai/service/linking"
	statussvc "github.com/viant/agently/genai/service/toolstatus"
	svc "github.com/viant/agently/genai/tool/service"
)

const Name = "llm/agents"

// Service exposes agent directory and execution as tool methods.
type Service struct {
	agent       *agentsvc.Service
	dirProvider func() []ListItem
	// Optional external runner: returns answer, status, taskID, contextID, streamSupported, warnings
	runExternal func(ctx context.Context, agentID, objective string, payload map[string]interface{}) (string, string, string, string, bool, []string, error)
	// Routing policy
	strict  bool
	allowed map[string]string // id -> source (internal|external)
	// Conversation/linking/status helpers
	conv   apiconv.Client
	linker *linksvc.Service
	status *statussvc.Service
}

// New creates a Service bound to the internal agent runtime.
type Option func(*Service)

func WithDirectoryProvider(f func() []ListItem) Option {
	return func(s *Service) { s.dirProvider = f }
}

// WithExternalRunner configures an external execution path resolver used when
// the agentId refers to an external A2A entry.
func WithExternalRunner(run func(ctx context.Context, agentID, objective string, payload map[string]interface{}) (answer, status, taskID, contextID string, streamSupported bool, warnings []string, err error)) Option {
	return func(s *Service) { s.runExternal = run }
}

// WithStrict enables strict directory routing: only ids present in the directory may be run.
func WithStrict(v bool) Option { return func(s *Service) { s.strict = v } }

// WithAllowedIDs configures the set of allowed agent ids (directory view).
func WithAllowedIDs(ids map[string]string) Option { return func(s *Service) { s.allowed = ids } }

// WithConversationClient injects the conversation client and initializes linking/status helpers.
func WithConversationClient(c apiconv.Client) Option {
	return func(s *Service) {
		s.conv = c
		if c != nil {
			s.linker = linksvc.New(c)
			s.status = statussvc.New(c)
		}
	}
}

func New(agent *agentsvc.Service, opts ...Option) *Service {
	s := &Service{agent: agent}
	for _, o := range opts {
		if o != nil {
			o(s)
		}
	}
	return s
}

// Name returns the service name.
func (s *Service) Name() string { return Name }

// Methods returns available methods.
func (s *Service) Methods() svc.Signatures {
	return []svc.Signature{
		{
			Name:        "list",
			Description: "List available agents for selection (filtered directory)",
			Input:       reflect.TypeOf(&struct{}{}),
			Output:      reflect.TypeOf(&ListOutput{}),
		},
		{
			Name:        "run",
			Description: "Run an agent by id with an objective and optional context",
			Input:       reflect.TypeOf(&RunInput{}),
			Output:      reflect.TypeOf(&RunOutput{}),
		},
	}
}

// Method resolves a method by name.
func (s *Service) Method(name string) (svc.Executable, error) {
	switch strings.ToLower(name) {
	case "list":
		return s.list, nil
	case "run":
		return s.run, nil
	default:
		return nil, svc.NewMethodNotFoundError(name)
	}
}

// list returns an empty directory for now. It will be populated in later phases
// with configured internal and external agent entries.
func (s *Service) list(ctx context.Context, in, out interface{}) error {
	// Accept either nil or empty struct as input
	lo, ok := out.(*ListOutput)
	if !ok {
		return svc.NewInvalidOutputError(out)
	}
	if s.dirProvider != nil {
		lo.Items = s.dirProvider()
		return nil
	}
	lo.Items = nil
	return nil
}

// run executes an internal agent synchronously via the agent runtime.
// External routing and streaming/status publishing will be added in later phases.
func (s *Service) run(ctx context.Context, in, out interface{}) error {
	ri, ok := in.(*RunInput)
	if !ok {
		return svc.NewInvalidInputError(in)
	}
	ro, ok := out.(*RunOutput)
	if !ok {
		return svc.NewInvalidOutputError(out)
	}
	// Strict routing: require id present in directory
	if s.strict {
		if _, ok := s.allowed[strings.TrimSpace(ri.AgentID)]; !ok {
			return svc.NewMethodNotFoundError("agent not registered in directory: " + strings.TrimSpace(ri.AgentID))
		}
	}
	// Resolve intended route when directory provided
	intended := ""
	if s.allowed != nil {
		if v, ok := s.allowed[strings.TrimSpace(ri.AgentID)]; ok {
			intended = v
		}
	}

	// Try external path when declared external or when allowed is unknown but external runner exists
	if s.runExternal != nil && (intended == "external" || intended == "") {
		var parent memory.TurnMeta
		if tm, ok := memory.TurnMetaFromContext(ctx); ok {
			parent = tm
		}
		childConvID := ""
		statusMsgID := ""
		// Create linked child convo under parent turn if available
		if s.linker != nil && strings.TrimSpace(parent.ConversationID) != "" {
			if cid, err := s.linker.CreateLinkedConversation(ctx, parent, false, nil); err == nil {
				childConvID = cid
				_ = s.linker.AddLinkMessage(ctx, parent, childConvID, "assistant", "tool", "exec")
				if s.status != nil {
					if mid, err := s.status.Start(ctx, parent, "llm/agents:run", "assistant", "tool", "exec"); err == nil {
						statusMsgID = mid
					}
				}
			}
		}
		// Prefer child conversation id as A2A contextId when present
		extCtx := ctx
		if strings.TrimSpace(childConvID) != "" {
			extCtx = memory.WithConversationID(ctx, childConvID)
		}
		ans, st, taskID, ctxID, streamSupp, warns, err := s.runExternal(extCtx, ri.AgentID, ri.Objective, ri.Context)
		if err != nil {
			if s.status != nil && strings.TrimSpace(statusMsgID) != "" && strings.TrimSpace(parent.ConversationID) != "" {
				_ = s.status.Finalize(ctx, parent, statusMsgID, "failed", "")
			}
			if intended == "external" {
				return err
			}
			// If route was unknown, fall through to internal path on error
		} else if taskID != "" || st != "" {
			ro.Answer = ans
			ro.Status = st
			ro.TaskID = taskID
			if strings.TrimSpace(ctxID) != "" {
				ro.ContextID = ctxID
			} else {
				ro.ContextID = childConvID
			}
			ro.StreamSupported = streamSupp
			ro.Warnings = append(ro.Warnings, warns...)
			if s.status != nil && strings.TrimSpace(statusMsgID) != "" && strings.TrimSpace(parent.ConversationID) != "" {
				preview := ans
				if len(preview) > 512 {
					preview = preview[:512]
				}
				_ = s.status.Finalize(ctx, parent, statusMsgID, strings.TrimSpace(st), preview)
			}
			return nil
		}
		// If we reach here: either external not declared (intended=="") and failed; try internal fallback.
	}
	if s.agent == nil {
		return svc.NewMethodNotFoundError("agent runtime not configured")
	}
	// Internal path via agent.Query. Conversation and user are derived from context by the service.
	// Attempt to create linked child conversation and status under the parent turn when available.
	var parent memory.TurnMeta
	if tm, ok := memory.TurnMetaFromContext(ctx); ok {
		parent = tm
	}
	childConvID := ""
	statusMsgID := ""
	if s.linker != nil && strings.TrimSpace(parent.ConversationID) != "" {
		if cid, err := s.linker.CreateLinkedConversation(ctx, parent, false, nil); err == nil {
			childConvID = cid
			// Add parent-side link message
			_ = s.linker.AddLinkMessage(ctx, parent, childConvID, "assistant", "tool", "exec")
			// Start status message
			if s.status != nil {
				if mid, err := s.status.Start(ctx, parent, "llm/agents:run", "assistant", "tool", "exec"); err == nil {
					statusMsgID = mid
				}
			}
		}
	}
	qi := &agentsvc.QueryInput{AgentID: ri.AgentID, Query: ri.Objective, Context: ri.Context}
	if strings.TrimSpace(childConvID) != "" {
		qi.ConversationID = childConvID
	}
	qo := &agentsvc.QueryOutput{}
	if err := s.agent.Query(ctx, qi, qo); err != nil {
		if s.status != nil && strings.TrimSpace(statusMsgID) != "" && strings.TrimSpace(parent.ConversationID) != "" {
			_ = s.status.Finalize(ctx, parent, statusMsgID, "failed", "")
		}
		return err
	}
	ro.Answer = qo.Content
	ro.Status = "succeeded"
	ro.ConversationID = qo.ConversationID
	ro.MessageID = qo.MessageID
	if s.status != nil && strings.TrimSpace(statusMsgID) != "" && strings.TrimSpace(parent.ConversationID) != "" {
		preview := qo.Content
		if len(preview) > 512 {
			preview = preview[:512]
		}
		_ = s.status.Finalize(ctx, parent, statusMsgID, "succeeded", preview)
	}
	return nil
}
