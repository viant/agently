package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"regexp"

	"github.com/google/uuid"
	chat "github.com/viant/agently/client/chat"
	agentmdl "github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/prompt"
	"github.com/viant/agently/genai/service/core"
	convw "github.com/viant/agently/pkg/agently/conversation/write"
)

// chainBinding is a minimal data holder exposed to chain query/when templates.
// ChainContext carries parent state and transcript to execute chain logic.
type ChainContext struct {
	Agent        *agentmdl.Agent
	Conversation *chat.Conversation
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
	// Always evaluate chains each turn; auto-next is still bounded
	// Reset per-turn counters unless this is an auto-next continuation turn
	isResume := false
	if v, ok := parent.Context["chain.resume"].(bool); ok && v {
		isResume = true
	}
	if !isResume {
		if parent.Context != nil {
			delete(parent.Context, "chain.depth")
		}
		// Reset per-conversation dedupe marks for chains
		if parent.Conversation != nil {
			_ = s.resetChainDedupe(ctx, parent.Conversation.Id)
		}
	}
	// Stamp status into context
	if parent.Context != nil {
		if _, ok := parent.Context["chain"]; !ok {
			parent.Context["chain"] = map[string]interface{}{}
		}
		cm := parent.Context["chain"].(map[string]interface{})
		cm["status"] = status
	}

	// Iterate chains in declaration order
	usedAutoNext := false
	for _, ch := range parent.Agent.Chains {
		if ch == nil {
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
		// Stamp minimal evaluation context
		if parent.Context != nil {
			if _, ok := parent.Context["chain"]; !ok {
				parent.Context["chain"] = map[string]interface{}{}
			}
			cm := parent.Context["chain"].(map[string]interface{})
			cm["status"] = status
			whenCtx := map[string]interface{}{
				"decision": shouldRunChain,
			}
			if ch.When != nil {
				whenCtx["expect"] = ch.When.Expect
			}
			if ch.When != nil && strings.TrimSpace(ch.When.Model) != "" {
				whenCtx["model"] = strings.TrimSpace(ch.When.Model)
			}
			cm["when"] = whenCtx
		}
		if !shouldRunChain {
			continue
		}
		policy := s.normalizePolicy(ch.Conversation)
		destConvID, err := s.ensureChildConversationIfNeeded(ctx, parent.ParentTurn, policy)
		if err != nil {
			return err
		}
		childIn := s.buildChildInputFromParent(parent, ch, on, destConvID)

		runSync := strings.EqualFold(strings.TrimSpace(ch.Mode), "sync") || strings.TrimSpace(ch.Mode) == ""
		if runSync {
			if err = s.runChainSync(ctx, childIn, ch, &parent, &usedAutoNext); err != nil {
				return err
			}
			continue
		}
		// async
		s.runChainAsync(ctx, childIn, ch, &parent)
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
	return false, s.conversation.PatchConversations(ctx, (*chat.MutableConversation)(&w))
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
	return s.conversation.PatchConversations(ctx, (*chat.MutableConversation)(&w))
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

func (s *Service) ensureChildConversationIfNeeded(ctx context.Context, parentTurn *memory.TurnMeta, policy string) (string, error) {
	if policy != "link" {
		return parentTurn.ConversationID, nil
	}
	destConvID := uuid.New().String()
	if s.conversation != nil {
		w := convw.Conversation{Has: &convw.ConversationHas{}}
		w.SetId(destConvID)
		w.SetVisibility(convw.VisibilityPublic)
		w.SetConversationParentId(parentTurn.ConversationID)
		w.SetConversationParentTurnId(parentTurn.TurnID)
		if err := s.conversation.PatchConversations(ctx, (*chat.MutableConversation)(&w)); err != nil {
			return "", err
		}
	}
	return destConvID, nil
}

func (s *Service) buildChildInputFromParent(parent ChainContext, ch *agentmdl.Chain, on string, destConvID string) *QueryInput {
	childIn := &QueryInput{
		ConversationID: destConvID,
		AgentID:        ch.Target.AgentID,
		UserId:         parent.UserID,
		Context:        map[string]interface{}{},
	}
	for k, v := range parent.Context {
		childIn.Context[k] = v
	}
	childIn.Context["chain"] = map[string]interface{}{
		"on":                   on,
		"targetAgentId":        ch.Target.AgentID,
		"policy":               s.normalizePolicy(ch.Conversation),
		"parentConversationId": parent.Conversation.Id,
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

func (s *Service) runChainSync(ctx context.Context, childIn *QueryInput, chain *agentmdl.Chain, parent *ChainContext, usedAutoNext *bool) error {

	if chain.Publish != nil {

		s.addMessage(ctx, parent.ParentTurn, "chain", "", "chaining", "", "")
	}

	content, role, err := s.fetchChainOutput(ctx, childIn, chain)
	if err != nil {
		if strings.ToLower(strings.TrimSpace(chain.OnError)) == "propagate" {
			return fmt.Errorf("chain target error: %w", err)
		}
		return nil
	}
	if strings.TrimSpace(content) == "" {
		return nil
	}
	// Continuation gate
	auto := chain.Publish != nil && chain.Publish.AutoNextTurn && role == "user" && !*usedAutoNext
	if auto {
		maxDepth := 10
		if chain.Limits != nil && chain.Limits.MaxDepth > 0 {
			maxDepth = chain.Limits.MaxDepth
		}
		depth := 0
		if v, ok := parent.Context["chain.depth"]; ok {
			switch t := v.(type) {
			case int:
				depth = t
			case float64:
				depth = int(t)
			}
		}
		// Dedupe
		if chain.Limits != nil && strings.TrimSpace(chain.Limits.DedupeKey) != "" {
			key := strings.TrimSpace(chain.Limits.DedupeKey)
			p := &prompt.Prompt{Text: key}
			b := s.buildPromptBindingFromParent(ctx, *parent, false)
			if exp, err := p.Generate(ctx, b); err == nil {
				dedupeKey := strings.TrimSpace(exp)
				if dedupeKey != "" {
					if seen, _ := s.seenAndMarkChainDedupe(ctx, parent.Conversation.Id, dedupeKey); seen {
						auto = false
					}
				}
			}
		}
		if auto && depth < maxDepth {
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
			next.Context["chain.resume"] = true
			next.Context["chain.depth"] = depth + 1
			next.Context["chain.parentTurnId"] = parent.ParentTurn.TurnID
			next.Context["chain.targetAgentId"] = chain.Target.AgentID
			var out QueryOutput
			if err := s.Query(ctx, next, &out); err != nil {
				return fmt.Errorf("continuation error: %w", err)
			}
			*usedAutoNext = true
			return nil
		}
	}
	// Publish-only fallback
	s.publishToParent(ctx, parent, role, chain, content)
	return nil
}

func (s *Service) runChainAsync(ctx context.Context, childIn *QueryInput, chian *agentmdl.Chain, parent *ChainContext) {
	go func(parentCtx context.Context, in *QueryInput) {
		base := context.Background()
		content, role, err := s.fetchChainOutput(base, in, chian)
		if err != nil {
			if strings.ToLower(strings.TrimSpace(chian.OnError)) == "message" {
				_, _ = s.addMessage(parentCtx, parent.ParentTurn, "assistant", "chain", "", "", "")
			}
			return
		}
		if strings.TrimSpace(content) == "" {
			return
		}
		s.publishToParent(parentCtx, parent, role, chian, content)
	}(ctx, childIn)
}

// fetchChainOutput executes a child chain query and returns trimmed content and resolved role.
// It centralizes shared logic for sync/async chain execution without applying error policies.
func (s *Service) fetchChainOutput(ctx context.Context, in *QueryInput, ch *agentmdl.Chain) (string, string, error) {
	var out QueryOutput
	if err := s.Query(ctx, in, &out); err != nil {
		return "", "", err
	}
	content := strings.TrimSpace(out.Content)
	role := "assistant"
	if ch != nil && ch.Publish != nil && ch.Publish.Role != "" {
		role = strings.ToLower(strings.TrimSpace(ch.Publish.Role))
		if role == "" {
			role = "assistant"
		}
	}
	return content, role, nil
}

func (s *Service) publishToParent(ctx context.Context, parent *ChainContext, role string, ch *agentmdl.Chain, content string) {
	parentSel := strings.ToLower(strings.TrimSpace(ch.Publish.Parent))
	if parentSel == "" {
		parentSel = "last_user"
	}
	aTurn := parent.ParentTurn
	switch parentSel {
	case "new_turn":
	//TODO create a new turn
	default:
	}
	actor := ""
	if role == "user" {
		actor = strings.TrimSpace(ch.Publish.Name)
	}
	_, _ = s.addMessage(ctx, aTurn, role, actor, content, "chain", "")
}
