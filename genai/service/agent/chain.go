package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"regexp"

	"github.com/google/uuid"
	apiconv "github.com/viant/agently/client/conversation"
	agentmdl "github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/prompt"
	"github.com/viant/agently/genai/service/core"
	convw "github.com/viant/agently/pkg/agently/conversation/write"
)

type chainControl struct {
	runs   map[string]int
	limits map[string]*agentmdl.ChainLimits
	sync.Mutex
}

func (c *chainControl) incrementRun(name string) {
	c.Lock()
	defer c.Unlock()
	c.runs[name]++
}

func (c *chainControl) ensureChainLimit(name string, limit *agentmdl.ChainLimits) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()
	if _, ok := c.limits[name]; ok {
		return
	}
	c.limits[name] = limit
}

func (c *chainControl) canRunChain(name string) bool {
	c.Lock()
	defer c.Unlock()
	if _, ok := c.limits[name]; !ok {
		return false
	}
	return c.runs[name] < c.limits[name].MaxDepth
}

type chainControlKeyType string

var chainControlKey = chainControlKeyType("chainControl")

func ensureChainControl(ctx context.Context) (*chainControl, context.Context) {
	value := ctx.Value(chainControlKey)
	if value == nil {
		ret := &chainControl{runs: make(map[string]int), limits: make(map[string]*agentmdl.ChainLimits)}
		return ret, context.WithValue(ctx, chainControlKey, ret)
	}
	return value.(*chainControl), ctx
}

type ChainContext struct {
	Agent        *agentmdl.Agent
	Conversation *apiconv.Conversation
	Context      map[string]interface{}
	UserID       string
	ParentTurn   *memory.TurnMeta
	Output       struct{ Content, Model, MessageID, Error string }
}

// NewChainContext builds a ChainContext from the current turn context,
// parent input and output. Conversation can be attached by the caller.
func NewChainContext(in *QueryInput, out *QueryOutput, turn *memory.TurnMeta) ChainContext {
	var cc ChainContext
	if in != nil {
		cc.Agent = in.Agent
		cc.Context = in.Context
		cc.UserID = in.UserId
	}

	cc.ParentTurn = turn

	if out != nil {
		cc.Output.Content = out.Content
		cc.Output.Model = out.Model
		cc.Output.MessageID = out.MessageID
	}
	return cc
}

// executeChains filters, evaluates and dispatches chains declared on the parent agent.
func (s *Service) executeChains(ctx context.Context, parent ChainContext, status string) error {
	if parent.Agent == nil || len(parent.Agent.Chains) == 0 {
		return nil
	}

	controls, ctx := ensureChainControl(ctx)

	for idx, ch := range parent.Agent.Chains {
		if ch == nil {
			continue
		}
		chainID := parent.ParentTurn.ConversationID + strconv.Itoa(idx) + ch.Target.AgentID
		controls.ensureChainLimit(chainID, ch.Limits)
		if !controls.canRunChain(chainID) {
			continue
		}
		on := strings.TrimSpace(strings.ToLower(ch.On))
		if on != "" && on != "*" && on != strings.ToLower(status) {
			continue
		}
		shouldRunChain, err := s.evalChainWhen(ctx, parent, ch.When)
		if err != nil {
			switch strings.ToLower(strings.TrimSpace(ch.OnError)) {
			case "propagate":
				return fmt.Errorf("chain when error: %w", err)
			}
			continue
		}
		if !shouldRunChain {
			continue
		}
		controls.incrementRun(chainID)
		policy := s.normalizePolicy(ch.Conversation)
		chainConversationID, err := s.ensureChainConversation(ctx, parent, policy)
		if err != nil {
			return err
		}
		childIn := s.buildQueryInput(parent, ch, on, chainConversationID)
		if err = s.runChainSync(ctx, childIn, ch, &parent); err != nil {
			return err
		}
	}
	return nil
}

func parseFloatSafe(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscan(s, &f)
	return f, err
}

// seenAndMarkChainDedupe checks if a dedupe key was already marked in conversation metadata;
// when not present, it records it and returns false. Best-effort; errors are returned but callers may ignore.
func (s *Service) seenAndMarkChainDedupe(ctx context.Context, convID, key string) (bool, error) {
	if s.conversation == nil || strings.TrimSpace(convID) == "" || strings.TrimSpace(key) == "" {
		return false, nil
	}
	cv, err := s.conversation.GetConversation(ctx, convID)
	if err != nil || cv == nil {
		return false, err
	}
	var meta ConversationMetadata
	if cv.Metadata != nil && strings.TrimSpace(*cv.Metadata) != "" {
		_ = json.Unmarshal([]byte(*cv.Metadata), &meta)
	}
	var seenSet map[string]struct{}
	if raw, ok := meta.Extra["chainSeen"]; ok && len(raw) > 0 {
		var arr []string
		if err := json.Unmarshal(raw, &arr); err == nil {
			seenSet = map[string]struct{}{}
			for _, v := range arr {
				seenSet[strings.TrimSpace(v)] = struct{}{}
			}
		}
	}
	if seenSet == nil {
		seenSet = map[string]struct{}{}
	}
	if _, ok := seenSet[key]; ok {
		return true, nil
	}
	seenSet[key] = struct{}{}
	arr := make([]string, 0, len(seenSet))
	for v := range seenSet {
		arr = append(arr, v)
	}
	b, err := json.Marshal(arr)
	if err != nil {
		return false, err
	}
	if meta.Extra == nil {
		meta.Extra = map[string]json.RawMessage{}
	}
	meta.Extra["chainSeen"] = b
	mbytes, err := json.Marshal(meta)
	if err != nil {
		return false, err
	}
	w := convw.Conversation{Has: &convw.ConversationHas{}}
	w.SetId(convID)
	w.SetMetadata(string(mbytes))
	return false, s.conversation.PatchConversations(ctx, (*apiconv.MutableConversation)(&w))
}

// resetChainDedupe clears per-conversation chain dedupe markers.
func (s *Service) resetChainDedupe(ctx context.Context, convID string) error {
	if s.conversation == nil || strings.TrimSpace(convID) == "" {
		return nil
	}
	cv, err := s.conversation.GetConversation(ctx, convID)
	if err != nil || cv == nil {
		return err
	}
	var meta ConversationMetadata
	if cv.Metadata != nil && strings.TrimSpace(*cv.Metadata) != "" {
		_ = json.Unmarshal([]byte(*cv.Metadata), &meta)
	}
	if meta.Extra == nil {
		meta.Extra = map[string]json.RawMessage{}
	}
	empty, _ := json.Marshal([]string{})
	meta.Extra["chainSeen"] = empty
	mbytes, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	w := convw.Conversation{Has: &convw.ConversationHas{}}
	w.SetId(convID)
	w.SetMetadata(string(mbytes))
	return s.conversation.PatchConversations(ctx, (*apiconv.MutableConversation)(&w))
}

// buildChainBindingFromParent deprecated; superseded by buildPromptBindingFromParent.

func (s *Service) evalChainWhen(ctx context.Context, parent ChainContext, spec *agentmdl.WhenSpec) (bool, error) {
	if spec == nil {
		return true, nil
	}
	b := s.buildPromptBindingFromParent(ctx, parent, true)

	// Expr path
	if strings.TrimSpace(spec.Expr) != "" {
		p := &prompt.Prompt{Text: spec.Expr}
		expanded, err := p.Generate(ctx, b)
		if err != nil {
			return false, err
		}
		sval := strings.TrimSpace(strings.ToLower(expanded))
		switch sval {
		case "", "false", "0", "no", "off":
			return false, nil
		case "true", "1", "yes", "on":
			return true, nil
		}
		if f, perr := parseFloatSafe(sval); perr == nil {
			return f != 0.0, nil
		}
		return true, nil
	}
	// LLM path
	if spec.Query == nil {
		return true, nil
	}
	// Build a minimal binding and attach last user/assistant only
	// Record interim user message with expanded query
	if err := spec.Query.Init(ctx); err != nil {
		return false, fmt.Errorf("when query init: %w", err)
	}
	expandedUser, err := spec.Query.Generate(ctx, b)
	if err != nil {
		return false, fmt.Errorf("when query generate: %w", err)
	}
	expandedUser = strings.TrimSpace(expandedUser)
	in := &core.GenerateInput{Prompt: spec.Query, Binding: b,
		UserID:         parent.UserID,
		ModelSelection: llm.ModelSelection{Options: &llm.Options{}},
	}
	if model := resolveWhenModel(spec, parent); model != "" {
		in.Model = model
	}
	in.Options.Mode = "chain"
	EnsureGenerateOptions(ctx, in, parent.Agent)
	var out core.GenerateOutput

	if err := s.llm.Generate(ctx, in, &out); err != nil {
		return false, fmt.Errorf("llm generate: %w", err)
	}
	resp := strings.TrimSpace(out.Content)
	// Expect evaluation
	kind := "boolean"
	if spec.Expect != nil && strings.TrimSpace(spec.Expect.Kind) != "" {
		kind = strings.ToLower(strings.TrimSpace(spec.Expect.Kind))
	}
	switch kind {
	case "regex":
		if spec.Expect == nil || strings.TrimSpace(spec.Expect.Pattern) == "" {
			return false, nil
		}
		re, err := regexp.Compile(spec.Expect.Pattern)
		if err != nil {
			return false, err
		}
		return re.MatchString(resp), nil
	case "jsonpath":
		if spec.Expect == nil || strings.TrimSpace(spec.Expect.Path) == "" {
			return false, nil
		}
		var obj interface{}
		if err := json.Unmarshal([]byte(resp), &obj); err != nil {
			return false, err
		}
		// minimal $.field support
		p := strings.TrimSpace(spec.Expect.Path)
		if strings.HasPrefix(p, "$.") {
			key := strings.TrimPrefix(p, "$.")
			if m, ok := obj.(map[string]interface{}); ok {
				v := m[key]
				switch t := v.(type) {
				case bool:
					return t, nil
				case string:
					s := strings.ToLower(strings.TrimSpace(t))
					return s == "true" || s == "1" || s == "yes" || s == "on", nil
				case float64:
					return t != 0, nil
				default:
					return v != nil, nil
				}
			}
		}
		return false, nil
	default: // boolean
		sval := strings.ToLower(resp)
		sval = strings.TrimSpace(sval)
		switch sval {
		case "", "false", "0", "no", "off":
			return false, nil
		case "true", "1", "yes", "on":
			return true, nil
		}
		if f, perr := parseFloatSafe(sval); perr == nil {
			return f != 0.0, nil
		}
		return true, nil
	}
}

// resolveWhenModel returns the model to use for WhenSpec evaluation.
// Priority: WhenSpec.Model > parent turn Output.Model > conversation default > agent model.
func resolveWhenModel(spec *agentmdl.WhenSpec, parent ChainContext) string {
	if spec != nil {
		if m := strings.TrimSpace(spec.Model); m != "" {
			return m
		}
	}
	if m := strings.TrimSpace(parent.Output.Model); m != "" {
		return m
	}
	if parent.Conversation != nil && parent.Conversation.DefaultModel != nil {
		if m := strings.TrimSpace(*parent.Conversation.DefaultModel); m != "" {
			return m
		}
	}
	if parent.Agent != nil {
		if m := strings.TrimSpace(parent.Agent.Model); m != "" {
			return m
		}
	}
	return ""
}

// buildPromptBindingFromParent builds a compact prompt.Binding from ChainContext.
// When minimal is true, only last user/assistant are attached to History.
func (s *Service) buildPromptBindingFromParent(ctx context.Context, parent ChainContext, lastTurnOnly bool) *prompt.Binding {
	b := &prompt.Binding{Context: map[string]interface{}{}}
	// Provide a compact context map including Inner Context and light meta
	b.Context = map[string]interface{}{
		"Context":      parent.Context,
		"Output":       parent.Output,
		"Agent":        struct{ ID, Name string }{ID: strings.TrimSpace(parent.Agent.ID), Name: strings.TrimSpace(parent.Agent.Name)},
		"Turn":         struct{ ConversationID, TurnID, ParentMessageID, Status string }{ConversationID: parent.Conversation.Id, TurnID: parent.ParentTurn.TurnID, ParentMessageID: parent.ParentTurn.ParentMessageID, Status: ""},
		"Conversation": struct{ ID, DefaultModel string }{ID: parent.Conversation.Id},
	}
	// Attach minimal history
	if parent.Conversation != nil {
		transcript := parent.Conversation.GetTranscript()
		b.History.Messages = transcript.History(lastTurnOnly)
	}
	return b
}

func (s *Service) normalizePolicy(policy string) string {
	p := strings.ToLower(strings.TrimSpace(policy))
	if p == "" {
		p = "link"
	}
	return p
}

func (s *Service) ensureChainConversation(ctx context.Context, chainCtx ChainContext, policy string) (string, error) {
	parentTurn := chainCtx.ParentTurn

	conversationID := parentTurn.ConversationID
	if policy == "link" {
		conversationID = uuid.New().String()

		conversation := convw.Conversation{Has: &convw.ConversationHas{}}
		conversation.SetId(conversationID)
		conversation.SetStatus("")
		conversation.SetVisibility(convw.VisibilityPublic)
		conversation.SetConversationParentId(parentTurn.ConversationID)
		conversation.SetConversationParentTurnId(parentTurn.TurnID)
		if err := s.conversation.PatchConversations(ctx, (*apiconv.MutableConversation)(&conversation)); err != nil {
			return "", fmt.Errorf("failed to create conversation: %w", err)
		}
		transcript := chainCtx.Conversation.GetTranscript().Last()
		err := s.cloneContextMessages(ctx, transcript, conversationID)
		if err != nil {
			return "", err
		}
	}
	return conversationID, nil
}

func (s *Service) cloneContextMessages(ctx context.Context, transcript apiconv.Transcript, conversationID string) error {
	transcriptTurnID := uuid.New().String()
	transcriptTurn := memory.TurnMeta{
		ParentMessageID: transcriptTurnID,
		TurnID:          transcriptTurnID,
		ConversationID:  conversationID,
	}
	if 1 == 1 {
		return nil
	}

	if err := s.startTurn(ctx, transcriptTurn); err != nil {
		return fmt.Errorf("failed to start transcript: %w", err)
	}
	for _, message := range transcript[0].GetMessages() {
		if message.Mode != nil && *message.Mode == "chain" {
			continue
		}
		mutable := message.NewMutable()
		mutable.SetId(uuid.New().String())
		mutable.SetTurnID(transcriptTurn.TurnID)
		mutable.SetConversationID(transcriptTurn.ConversationID)
		mutable.SetParentMessageID(transcriptTurn.ParentMessageID)
		if err := s.conversation.PatchMessage(ctx, mutable); err != nil {
			return fmt.Errorf("failed to patch transcript message: %w", err)
		}
	}
	return nil
}

func (s *Service) buildQueryInput(parent ChainContext, ch *agentmdl.Chain, on string, chainConversationID string) *QueryInput {
	childIn := &QueryInput{
		ParentConversationID: parent.ParentTurn.ConversationID,
		ConversationID:       chainConversationID,
		AgentID:              ch.Target.AgentID,
		UserId:               parent.UserID,
		Context:              map[string]interface{}{},
	}
	for k, v := range parent.Context {
		childIn.Context[k] = v
	}

	if ch.Query != nil {
		b := s.buildPromptBindingFromParent(context.Background(), parent, false)
		if err := ch.Query.Init(context.Background()); err == nil {
			if q, err := ch.Query.Generate(context.Background(), b); err == nil {
				childIn.Query = q
			}
		}
	}
	return childIn
}

func (s *Service) runChainSync(ctx context.Context, childIn *QueryInput, chain *agentmdl.Chain, parent *ChainContext) error {
	publish := chain.Publish
	if publish == nil {
		chain.Publish = &agentmdl.ChainPublish{
			Role: "user",
		}
	}
	role := chain.Publish.Role
	if role == "" {
		role = "assistant"
	}
	actor := chain.Publish.Name
	if actor == "" {
		actor = "chain"
	}

	if _, err := apiconv.AddMessage(ctx, s.conversation, parent.ParentTurn,
		apiconv.WithId(uuid.New().String()),
		apiconv.WithRole(role),
		apiconv.WithInterim(1),
		apiconv.WithContent(""),
		apiconv.WithCreatedByUserID(actor),
		apiconv.WithMode("chain"),
		apiconv.WithLinkedConversationID(childIn.ConversationID)); err != nil {
		return err
	}

	content, err := s.fetchChainOutput(ctx, childIn, chain)
	if err != nil {
		if strings.ToLower(strings.TrimSpace(chain.OnError)) == "propagate" {
			return fmt.Errorf("chain target error: %w", err)
		}
		return nil
	}
	if strings.TrimSpace(content) == "" {
		return nil
	}
	// Continue parent as new user turn
	next := &QueryInput{
		ConversationID: parent.Conversation.Id,
		AgentID:        parent.Agent.ID,
		UserId:         strings.TrimSpace(chain.Publish.Name),
		Query:          content,
		Context:        map[string]interface{}{},
	}
	for k, v := range parent.Context {
		next.Context[k] = v
	}
	var out QueryOutput
	if err := s.Query(ctx, next, &out); err != nil {
		return fmt.Errorf("continuation error: %w", err)
	}
	return nil

}

// fetchChainOutput executes a child chain query and returns trimmed content and resolved role.
// It centralizes shared logic for sync/async chain execution without applying error policies.
func (s *Service) fetchChainOutput(ctx context.Context, in *QueryInput, ch *agentmdl.Chain) (string, error) {
	var out QueryOutput
	if err := s.Query(ctx, in, &out); err != nil {
		return "", fmt.Errorf("failed to run query %w", err)
	}
	content := strings.TrimSpace(out.Content)
	return content, nil
}
