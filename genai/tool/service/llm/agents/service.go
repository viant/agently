package agents

import (
	"context"
	"reflect"
	"strings"
	"time"

	apiconv "github.com/viant/agently/client/conversation"
	agentmdl "github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/memory"
	agentsvc "github.com/viant/agently/genai/service/agent"
	linksvc "github.com/viant/agently/genai/service/linking"
	statussvc "github.com/viant/agently/genai/service/toolstatus"
	toolpol "github.com/viant/agently/genai/tool"
	svc "github.com/viant/agently/genai/tool/service"
	agconv "github.com/viant/agently/pkg/agently/conversation"
	convw "github.com/viant/agently/pkg/agently/conversation/write"
	"github.com/viant/agently/shared"
)

const Name = "llm/agents"

// agentRuntime abstracts the subset of the agent service used by this
// tool, allowing unit tests to inject a lightweight fake.
type agentRuntime interface {
	Query(ctx context.Context, input *agentsvc.QueryInput, output *agentsvc.QueryOutput) error
	Finder() agentmdl.Finder
}

// Service exposes agent directory and execution as tool methods.
type Service struct {
	agent       agentRuntime
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

// ToolTimeout suggests a larger timeout for llm/agents service tools which run
// full agent turns.
func (s *Service) ToolTimeout() time.Duration { return 10 * time.Minute }

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
			Name:        "me",
			Description: "Return conversation id, agent name, and model used for the current context",
			Input:       reflect.TypeOf(&struct{}{}),
			Output:      reflect.TypeOf(&MeOutput{}),
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
	case "me":
		return s.me, nil
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

// me returns the current conversation id, agent name, and model used (best-effort).
func (s *Service) me(ctx context.Context, in, out interface{}) error {
	mo, ok := out.(*MeOutput)
	if !ok {
		return svc.NewInvalidOutputError(out)
	}
	mo.ConversationID = strings.TrimSpace(memory.ConversationIDFromContext(ctx))
	// Best-effort: load conversation to get agent id + model
	if s.conv != nil && mo.ConversationID != "" {
		if c, err := s.conv.GetConversation(ctx, mo.ConversationID); err == nil && c != nil {
			if c.AgentId != nil && strings.TrimSpace(*c.AgentId) != "" {
				if s.agent != nil && s.agent.Finder() != nil {
					if ag, err := s.agent.Finder().Find(ctx, strings.TrimSpace(*c.AgentId)); err == nil && ag != nil && ag.Profile != nil {
						mo.AgentName = strings.TrimSpace(ag.Profile.Name)
					}
				}
				if mo.AgentName == "" {
					mo.AgentName = strings.TrimSpace(*c.AgentId)
				}
			}
			if c.DefaultModel != nil && strings.TrimSpace(*c.DefaultModel) != "" {
				mo.Model = strings.TrimSpace(*c.DefaultModel)
			}
		}
	}
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
	convID := strings.TrimSpace(memory.ConversationIDFromContext(ctx))
	if convID == "" {
		if v := strings.TrimSpace(ri.ConversationID); v != "" {
			convID = v
			ctx = memory.WithConversationID(ctx, convID)
		}
	}
	ro.ConversationID = convID
	debugf("agents.run start convo=%q agent_id=%q objective_len=%d objective_head=%q objective_tail=%q context_keys=%d", strings.TrimSpace(convID), strings.TrimSpace(ri.AgentID), len(ri.Objective), headString(ri.Objective, 512), tailString(ri.Objective, 512), len(ri.Context))
	// Strict routing: require id present in directory
	if s.strict {
		if _, ok := s.allowed[strings.TrimSpace(ri.AgentID)]; !ok {
			errorf("agents.run strict reject agent_id=%q", strings.TrimSpace(ri.AgentID))
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
	debugf("agents.run routing agent_id=%q intended=%q", strings.TrimSpace(ri.AgentID), strings.TrimSpace(intended))

	// Default to internal when the agent is resolvable locally; only fall back to
	// external when explicitly routed or when the agent id is not found internally.
	internalKnown := s.isInternalAgent(ctx, strings.TrimSpace(ri.AgentID))
	debugf("agents.run route check agent_id=%q internal_known=%v external_enabled=%v", strings.TrimSpace(ri.AgentID), internalKnown, s.runExternal != nil)
	if s.runExternal != nil && (intended == "external" || (intended == "" && !internalKnown)) {
		var parent memory.TurnMeta
		if tm, ok := memory.TurnMetaFromContext(ctx); ok {
			parent = tm
		}
		debugf("agents.run external path parent_convo=%q parent_turn=%q", strings.TrimSpace(parent.ConversationID), strings.TrimSpace(parent.TurnID))
		childConvID := ""
		statusMsgID := ""

		// Reuse existing child conversation based on agent profile scope; otherwise create & link
		if s.linker != nil && strings.TrimSpace(parent.ConversationID) != "" {
			if s.conv != nil && strings.TrimSpace(ri.AgentID) != "" {
				scope := "new"
				if s.agent != nil && s.agent.Finder() != nil {
					if ag, err := s.agent.Finder().Find(ctx, strings.TrimSpace(ri.AgentID)); err == nil && ag != nil && ag.Profile != nil {
						v := strings.ToLower(strings.TrimSpace(ag.Profile.ConversationScope))
						if v == "parent" || v == "parentturn" || v == "new" {
							scope = v
						}
					}
				}
				debugf("agents.run external scope agent_id=%q scope=%q", strings.TrimSpace(ri.AgentID), strings.TrimSpace(scope))
				if scope != "new" {
					in := &agconv.ConversationInput{
						AgentId:          ri.AgentID,
						ParentId:         parent.ConversationID,
						DefaultPredicate: "1",
						Has:              &agconv.ConversationInputHas{AgentId: true, ParentId: true, DefaultPredicate: true},
					}
					if scope == "parentturn" {
						in.ParentTurnId = parent.TurnID
						in.Has.ParentTurnId = true
					}
					debugf("agents.run external reuse lookup agent_id=%q parent_convo=%q parent_turn=%q scope=%q", strings.TrimSpace(ri.AgentID), strings.TrimSpace(parent.ConversationID), strings.TrimSpace(parent.TurnID), strings.TrimSpace(scope))
				}
			}
			if strings.TrimSpace(childConvID) == "" {
				if cid, err := s.linker.CreateLinkedConversation(ctx, parent, false, nil); err == nil {
					childConvID = cid
					debugf("agents.run external created child_convo=%q parent_convo=%q", strings.TrimSpace(childConvID), strings.TrimSpace(parent.ConversationID))
					// Set agent id on newly created conversation
					if s.conv != nil && strings.TrimSpace(ri.AgentID) != "" {
						upd := convw.Conversation{Has: &convw.ConversationHas{}}
						upd.SetId(childConvID)
						upd.SetAgentId(strings.TrimSpace(ri.AgentID))
						if perr := s.conv.PatchConversations(ctx, (*apiconv.MutableConversation)(&upd)); perr != nil {
							errorf("agents.run external set agent error child_convo=%q agent_id=%q err=%v", strings.TrimSpace(childConvID), strings.TrimSpace(ri.AgentID), perr)
						}
					}
					// Include a compact objective preview in the link message for traceability.
					preview := shared.RuneTruncate(strings.TrimSpace(ri.Objective), 512)
					if lerr := s.linker.AddLinkMessage(ctx, parent, childConvID, "assistant", "tool", "exec", preview); lerr != nil {
						errorf("agents.run external link message error child_convo=%q err=%v", strings.TrimSpace(childConvID), lerr)
					}
				} else {
					errorf("agents.run external create child error parent_convo=%q err=%v", strings.TrimSpace(parent.ConversationID), err)
				}
			}
			// Always record a status for this parent step
			if s.status != nil {
				if mid, err := s.status.Start(ctx, parent, "llm/agents:run", "assistant", "tool", "exec"); err == nil {
					statusMsgID = mid
					debugf("agents.run external status start parent_convo=%q message_id=%q", strings.TrimSpace(parent.ConversationID), strings.TrimSpace(statusMsgID))
				} else if err != nil {
					errorf("agents.run external status start error parent_convo=%q err=%v", strings.TrimSpace(parent.ConversationID), err)
				}
			}
		}

		// Prefer child conversation id as A2A contextId when present
		extCtx := ctx
		if strings.TrimSpace(childConvID) != "" {
			extCtx = memory.WithConversationID(ctx, childConvID)
			ro.ConversationID = childConvID
		}
		debugf("agents.run external invoke agent_id=%q child_convo=%q", strings.TrimSpace(ri.AgentID), strings.TrimSpace(childConvID))
		ans, st, taskID, ctxID, streamSupp, warns, err := s.runExternal(extCtx, ri.AgentID, ri.Objective, ri.Context)
		if err != nil {
			errorf("agents.run external error agent_id=%q child_convo=%q err=%v", strings.TrimSpace(ri.AgentID), strings.TrimSpace(childConvID), err)
			if s.status != nil && strings.TrimSpace(statusMsgID) != "" && strings.TrimSpace(parent.ConversationID) != "" {
				_ = s.status.Finalize(ctx, parent, statusMsgID, "failed", "")
			}
			if intended == "external" {
				return err
			}
			// If route was unknown, fall through to internal path on error
		} else if taskID != "" || st != "" {
			debugf("agents.run external ok agent_id=%q child_convo=%q status=%q task_id=%q context_id=%q", strings.TrimSpace(ri.AgentID), strings.TrimSpace(childConvID), strings.TrimSpace(st), strings.TrimSpace(taskID), strings.TrimSpace(ctxID))
			ro.Answer = ans
			ro.Status = st
			ro.TaskID = taskID
			if ro.ConversationID == "" {
				ro.ConversationID = strings.TrimSpace(memory.ConversationIDFromContext(extCtx))
			}
			if strings.TrimSpace(ctxID) != "" {
				ro.ContextID = ctxID
			} else {
				ro.ContextID = childConvID
			}
			ro.StreamSupported = streamSupp
			ro.Warnings = append(ro.Warnings, warns...)
			if s.status != nil && strings.TrimSpace(statusMsgID) != "" && strings.TrimSpace(parent.ConversationID) != "" {
				preview := shared.RuneTruncate(ans, 512)
				_ = s.status.Finalize(ctx, parent, statusMsgID, strings.TrimSpace(st), preview)
			}
			return nil
		}
		// If we reach here: either external not declared (intended=="") and failed; try internal fallback.
	}
	if s.agent == nil {
		errorf("agents.run internal error: agent runtime not configured")
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
		// Determine conversation scope from agent profile (default: new)
		scope := "new"
		if s.agent != nil && s.agent.Finder() != nil && strings.TrimSpace(ri.AgentID) != "" {
			if ag, err := s.agent.Finder().Find(ctx, strings.TrimSpace(ri.AgentID)); err == nil && ag != nil && ag.Profile != nil {
				v := strings.ToLower(strings.TrimSpace(ag.Profile.ConversationScope))
				if v == "parent" || v == "parentturn" || v == "new" {
					scope = v
				}
			}
		}
		debugf("agents.run internal scope agent_id=%q scope=%q", strings.TrimSpace(ri.AgentID), strings.TrimSpace(scope))
		// Reuse based on scope unless "new"
		if scope != "new" && s.conv != nil && strings.TrimSpace(ri.AgentID) != "" {
			input := &agconv.ConversationInput{
				AgentId:          ri.AgentID,
				ParentId:         parent.ConversationID,
				DefaultPredicate: "1",
				Has:              &agconv.ConversationInputHas{AgentId: true, ParentId: true, DefaultPredicate: true},
			}
			if scope == "parentturn" {
				input.ParentTurnId = parent.TurnID
				input.Has.ParentTurnId = true
			}
			debugf("agents.run internal reuse lookup agent_id=%q parent_convo=%q parent_turn=%q scope=%q", strings.TrimSpace(ri.AgentID), strings.TrimSpace(parent.ConversationID), strings.TrimSpace(parent.TurnID), strings.TrimSpace(scope))
		}
		if strings.TrimSpace(childConvID) == "" {
			if cid, err := s.linker.CreateLinkedConversation(ctx, parent, false, nil); err == nil {
				childConvID = cid
				debugf("agents.run internal created child_convo=%q parent_convo=%q", strings.TrimSpace(childConvID), strings.TrimSpace(parent.ConversationID))
				// Populate agent id on the new conversation when available
				if s.conv != nil && strings.TrimSpace(ri.AgentID) != "" {
					upd := convw.Conversation{Has: &convw.ConversationHas{}}
					upd.SetId(childConvID)
					upd.SetAgentId(strings.TrimSpace(ri.AgentID))
					if perr := s.conv.PatchConversations(ctx, (*apiconv.MutableConversation)(&upd)); perr != nil {
						errorf("agents.run internal set agent error child_convo=%q agent_id=%q err=%v", strings.TrimSpace(childConvID), strings.TrimSpace(ri.AgentID), perr)
					}
				}
				// Add parent-side link message with objective preview
				preview := shared.RuneTruncate(strings.TrimSpace(ri.Objective), 512)
				if lerr := s.linker.AddLinkMessage(ctx, parent, childConvID, "assistant", "tool", "exec", preview); lerr != nil {
					errorf("agents.run internal link message error child_convo=%q err=%v", strings.TrimSpace(childConvID), lerr)
				}
			} else {
				errorf("agents.run internal create child error parent_convo=%q err=%v", strings.TrimSpace(parent.ConversationID), err)
			}
		}
		// Start status message
		if s.status != nil {
			if mid, err := s.status.Start(ctx, parent, "llm/agents:run", "assistant", "tool", "exec"); err == nil {
				statusMsgID = mid
				debugf("agents.run internal status start parent_convo=%q message_id=%q", strings.TrimSpace(parent.ConversationID), strings.TrimSpace(statusMsgID))
			} else if err != nil {
				errorf("agents.run internal status start error parent_convo=%q err=%v", strings.TrimSpace(parent.ConversationID), err)
			}
		}
	}
	qi := &agentsvc.QueryInput{AgentID: ri.AgentID, Query: ri.Objective, Context: ri.Context}
	// llm/agents:run should honor the delegated agent's configured tools (patterns/bundles)
	qi.ToolsAllowed = []string{}
	if ri.ModelPreferences != nil {
		qi.ModelPreferences = ri.ModelPreferences
	}
	// Thread through optional reasoning effort override when provided.
	if ri.ReasoningEffort != nil {
		qi.ReasoningEffort = ri.ReasoningEffort
	}
	if strings.TrimSpace(childConvID) != "" {
		qi.ConversationID = childConvID
		ro.ConversationID = childConvID
	}
	qo := &agentsvc.QueryOutput{}
	// Clear any parent tool policy from context to avoid restricting delegated runs.
	childCtx := toolpol.WithPolicy(ctx, nil)
	debugf("agents.run internal invoke agent_id=%q child_convo=%q", strings.TrimSpace(ri.AgentID), strings.TrimSpace(childConvID))
	if err := s.agent.Query(childCtx, qi, qo); err != nil {
		errorf("agents.run internal error agent_id=%q child_convo=%q err=%v", strings.TrimSpace(ri.AgentID), strings.TrimSpace(childConvID), err)
		if s.status != nil && strings.TrimSpace(statusMsgID) != "" && strings.TrimSpace(parent.ConversationID) != "" {
			_ = s.status.Finalize(ctx, parent, statusMsgID, "failed", "")
		}
		return err
	}
	debugf("agents.run internal ok agent_id=%q child_convo=%q message_id=%q", strings.TrimSpace(ri.AgentID), strings.TrimSpace(childConvID), strings.TrimSpace(qo.MessageID))
	ro.Answer = qo.Content
	ro.Status = "succeeded"
	if strings.TrimSpace(qo.ConversationID) != "" {
		ro.ConversationID = qo.ConversationID
	}
	if ro.ConversationID == "" {
		ro.ConversationID = convID
	}
	ro.MessageID = qo.MessageID
	if s.status != nil && strings.TrimSpace(statusMsgID) != "" && strings.TrimSpace(parent.ConversationID) != "" {
		preview := shared.RuneTruncate(qo.Content, 512)
		_ = s.status.Finalize(ctx, parent, statusMsgID, "succeeded", preview)
	}
	debugf("agents.run done convo=%q agent_id=%q status=%q", strings.TrimSpace(ro.ConversationID), strings.TrimSpace(ri.AgentID), strings.TrimSpace(ro.Status))
	return nil
}

func (s *Service) isInternalAgent(ctx context.Context, agentID string) bool {
	if s == nil || s.agent == nil || strings.TrimSpace(agentID) == "" {
		return false
	}
	// Handle typed-nil interfaces (e.g. var x *T=nil; interface{...}=x).
	if v := reflect.ValueOf(s.agent); v.Kind() == reflect.Pointer && v.IsNil() {
		return false
	}
	if s.agent.Finder() == nil {
		return false
	}
	ag, err := s.agent.Finder().Find(ctx, strings.TrimSpace(agentID))
	return err == nil && ag != nil
}
