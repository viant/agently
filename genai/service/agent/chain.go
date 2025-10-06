package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	apiconv "github.com/viant/agently/client/conversation"
	agentmdl "github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/prompt"
	"github.com/viant/agently/genai/usage"
	convw "github.com/viant/agently/pkg/agently/conversation/write"
)

// chainBinding is a minimal data holder exposed to chain query/when templates.
type chainBinding struct {
	Output struct {
		Content   string
		Model     string
		MessageID string
		// Error message when parent failed; empty otherwise
		Error string
	}
	Usage        *usage.Aggregator
	Context      map[string]interface{}
	Agent        struct{ ID, Name string }
	Turn         struct{ ConversationID, TurnID, ParentMessageID, Status string }
	Conversation struct{ ID, DefaultModel string }
}

// executeChains filters, evaluates and dispatches chains declared on the parent agent.
func (s *Service) executeChains(ctx context.Context, parentIn *QueryInput, parentOut *QueryOutput, status string) error {
	if parentIn == nil || parentIn.Agent == nil || len(parentIn.Agent.Chains) == 0 {
		return nil
	}
	// Skip chaining when this is a continuation turn to avoid loops unless explicitly allowed
	if v, ok := parentIn.Context["chain.resume"].(bool); ok && v {
		return nil
	}
	bind := s.buildChainBinding(ctx, parentIn, parentOut, status)

	// Iterate chains in declaration order
	usedAutoNext := false
	for _, ch := range parentIn.Agent.Chains {
		if ch == nil {
			continue
		}
		on := strings.TrimSpace(strings.ToLower(ch.On))
		if on != "" && on != "*" && on != strings.ToLower(status) {
			continue
		}
		ok, wErr := s.evalChainWhen(ctx, bind, ch.When)
		if wErr != nil {
			switch strings.ToLower(strings.TrimSpace(ch.OnError)) {
			case "propagate":
				return fmt.Errorf("chain when error: %w", wErr)
			case "message":
				_, _ = s.addMessage(ctx, parentIn.ConversationID, "assistant", "", fmt.Sprintf("chain when-eval error: %v", wErr), "", bind.Turn.TurnID)
			}
			continue
		}
		if !ok {
			continue
		}
		policy := s.normalizePolicy(ch.Conversation)
		destConvID, err := s.ensureChildConversationIfNeeded(ctx, parentIn.ConversationID, bind.Turn.TurnID, policy)
		if err != nil {
			return err
		}
		childIn := s.buildChildInput(parentIn, ch, on, destConvID, bind)

		runSync := strings.EqualFold(strings.TrimSpace(ch.Mode), "sync") || strings.TrimSpace(ch.Mode) == ""
		if runSync {
			if err := s.runChainSync(ctx, childIn, ch, parentIn, &bind, &usedAutoNext); err != nil {
				return err
			}
			continue
		}
		// async
		s.runChainAsync(ctx, childIn, ch, parentIn, &bind)
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

// --- helpers ---

func (s *Service) buildChainBinding(ctx context.Context, parentIn *QueryInput, parentOut *QueryOutput, status string) chainBinding {
	var bind chainBinding
	bind.Output.Content = parentOut.Content
	bind.Output.Model = parentOut.Model
	bind.Output.MessageID = parentOut.MessageID
	if strings.EqualFold(status, "failed") {
		bind.Output.Error = ""
	}
	bind.Usage = parentOut.Usage
	if parentIn != nil && parentIn.Agent != nil {
		bind.Agent.ID = parentIn.Agent.ID
		bind.Agent.Name = parentIn.Agent.Name
	}
	if tm, ok := memory.TurnMetaFromContext(ctx); ok {
		bind.Turn.ConversationID = tm.ConversationID
		bind.Turn.TurnID = tm.TurnID
		bind.Turn.ParentMessageID = tm.ParentMessageID
	} else if parentIn != nil {
		bind.Turn.ConversationID = parentIn.ConversationID
	}
	bind.Turn.Status = status
	if parentIn != nil {
		bind.Conversation.ID = parentIn.ConversationID
		bind.Context = map[string]interface{}{}
		for k, v := range parentIn.Context {
			bind.Context[k] = v
		}
	}
	return bind
}

func (s *Service) evalChainWhen(ctx context.Context, bind chainBinding, expr string) (bool, error) {
	if strings.TrimSpace(expr) == "" {
		return true, nil
	}
	p := &prompt.Prompt{Text: expr}
	b := &prompt.Binding{Context: map[string]interface{}{
		"Output":       bind.Output,
		"Usage":        bind.Usage,
		"Context":      bind.Context,
		"Agent":        bind.Agent,
		"Turn":         bind.Turn,
		"Conversation": bind.Conversation,
	}}
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

func (s *Service) normalizePolicy(policy string) string {
	p := strings.ToLower(strings.TrimSpace(policy))
	if p == "" {
		p = "link"
	}
	return p
}

func (s *Service) ensureChildConversationIfNeeded(ctx context.Context, parentConvID, parentTurnID, policy string) (string, error) {
	if policy != "link" {
		return parentConvID, nil
	}
	destConvID := uuid.New().String()
	if s.conversation != nil {
		w := convw.Conversation{Has: &convw.ConversationHas{}}
		w.SetId(destConvID)
		w.SetVisibility(convw.VisibilityPublic)
		w.SetConversationParentId(parentConvID)
		if strings.TrimSpace(parentTurnID) != "" {
			w.SetConversationParentTurnId(parentTurnID)
		}
		if err := s.conversation.PatchConversations(ctx, (*apiconv.MutableConversation)(&w)); err != nil {
			return "", err
		}
	}
	return destConvID, nil
}

func (s *Service) buildChildInput(parentIn *QueryInput, ch *agentmdl.Chain, on string, destConvID string, bind chainBinding) *QueryInput {
	childIn := &QueryInput{
		ConversationID: destConvID,
		AgentID:        ch.Target.AgentID,
		UserId:         parentIn.UserId,
		Context:        map[string]interface{}{},
	}
	for k, v := range parentIn.Context {
		childIn.Context[k] = v
	}
	childIn.Context["chain"] = map[string]interface{}{
		"on":                   on,
		"targetAgentId":        ch.Target.AgentID,
		"policy":               s.normalizePolicy(ch.Conversation),
		"parentConversationId": parentIn.ConversationID,
	}
	if ch.Query != nil {
		b := &prompt.Binding{Context: map[string]interface{}{
			"Output":       bind.Output,
			"Usage":        bind.Usage,
			"Context":      bind.Context,
			"Agent":        bind.Agent,
			"Turn":         bind.Turn,
			"Conversation": bind.Conversation,
		}}
		if err := ch.Query.Init(context.Background()); err == nil {
			if q, err := ch.Query.Generate(context.Background(), b); err == nil {
				childIn.Query = q
			}
		}
	}
	return childIn
}

func (s *Service) runChainSync(ctx context.Context, childIn *QueryInput, ch *agentmdl.Chain, parentIn *QueryInput, bind *chainBinding, usedAutoNext *bool) error {
	var childOut QueryOutput
	if err := s.Query(ctx, childIn, &childOut); err != nil {
		switch strings.ToLower(strings.TrimSpace(ch.OnError)) {
		case "propagate":
			return fmt.Errorf("chain target error: %w", err)
		case "message":
			_, _ = s.addMessage(ctx, parentIn.ConversationID, "assistant", "", fmt.Sprintf("chain error: %v", err), "", bind.Turn.TurnID)
		}
		return nil
	}
	content := strings.TrimSpace(childOut.Content)
	if content == "" {
		return nil
	}
	role := strings.ToLower(strings.TrimSpace(ch.Publish.Role))
	if role == "" {
		role = "assistant"
	}
	// Continuation gate
	auto := ch.Publish != nil && ch.Publish.AutoNextTurn && role == "user" && !*usedAutoNext
	if auto {
		maxDepth := 10
		if ch.Limits != nil && ch.Limits.MaxDepth > 0 {
			maxDepth = ch.Limits.MaxDepth
		}
		depth := 0
		if v, ok := parentIn.Context["chain.depth"]; ok {
			switch t := v.(type) {
			case int:
				depth = t
			case float64:
				depth = int(t)
			}
		}
		// Dedupe
		if ch.Limits != nil && strings.TrimSpace(ch.Limits.DedupeKey) != "" {
			key := strings.TrimSpace(ch.Limits.DedupeKey)
			p := &prompt.Prompt{Text: key}
			b := &prompt.Binding{Context: map[string]interface{}{
				"Output":       bind.Output,
				"Usage":        bind.Usage,
				"Context":      bind.Context,
				"Agent":        bind.Agent,
				"Turn":         bind.Turn,
				"Conversation": bind.Conversation,
			}}
			if exp, err := p.Generate(ctx, b); err == nil {
				dedupeKey := strings.TrimSpace(exp)
				if dedupeKey != "" {
					if seen, _ := s.seenAndMarkChainDedupe(ctx, parentIn.ConversationID, dedupeKey); seen {
						auto = false
					}
				}
			}
		}
		if auto && depth < maxDepth {
			// Continue parent as new user turn
			next := &QueryInput{
				ConversationID: parentIn.ConversationID,
				AgentID:        parentIn.Agent.ID,
				UserId:         strings.TrimSpace(ch.Publish.Name),
				Query:          content,
				Context:        map[string]interface{}{},
			}
			if next.UserId == "" {
				next.UserId = parentIn.UserId
			}
			for k, v := range parentIn.Context {
				next.Context[k] = v
			}
			next.Context["chain.resume"] = true
			next.Context["chain.depth"] = depth + 1
			next.Context["chain.parentTurnId"] = bind.Turn.TurnID
			next.Context["chain.targetAgentId"] = ch.Target.AgentID
			var out QueryOutput
			if err := s.Query(ctx, next, &out); err != nil {
				return fmt.Errorf("continuation error: %w", err)
			}
			*usedAutoNext = true
			return nil
		}
	}
	// Publish-only fallback
	s.publishToParent(ctx, parentIn, bind, role, ch, content)
	return nil
}

func (s *Service) runChainAsync(ctx context.Context, childIn *QueryInput, ch *agentmdl.Chain, parentIn *QueryInput, bind *chainBinding) {
	go func(parentCtx context.Context, in *QueryInput) {
		base := context.Background()
		var out QueryOutput
		if err := s.Query(base, in, &out); err != nil {
			if strings.ToLower(strings.TrimSpace(ch.OnError)) == "message" {
				_, _ = s.addMessage(parentCtx, parentIn.ConversationID, "assistant", "", fmt.Sprintf("chain error: %v", err), "", bind.Turn.TurnID)
			}
			return
		}
		content := strings.TrimSpace(out.Content)
		if content == "" {
			return
		}
		role := strings.ToLower(strings.TrimSpace(ch.Publish.Role))
		if role == "" {
			role = "assistant"
		}
		s.publishToParent(parentCtx, parentIn, bind, role, ch, content)
	}(ctx, childIn)
}

func (s *Service) publishToParent(ctx context.Context, parentIn *QueryInput, bind *chainBinding, role string, ch *agentmdl.Chain, content string) {
	parentSel := strings.ToLower(strings.TrimSpace(ch.Publish.Parent))
	if parentSel == "" {
		parentSel = "last_user"
	}
	parentID := ""
	switch parentSel {
	case "same_turn":
		parentID = bind.Turn.TurnID
	case "last_user":
		parentID = bind.Turn.ParentMessageID
	}
	actor := ""
	if role == "user" {
		actor = strings.TrimSpace(ch.Publish.Name)
		if actor == "" {
			actor = parentIn.UserId
		}
	}
	_, _ = s.addMessage(ctx, parentIn.ConversationID, role, actor, content, "", parentID)
}
