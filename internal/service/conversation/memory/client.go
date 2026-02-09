package memory

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	convcli "github.com/viant/agently/client/conversation"
	authctx "github.com/viant/agently/internal/auth"
	agconv "github.com/viant/agently/pkg/agently/conversation"
	msgw "github.com/viant/agently/pkg/agently/message/write"
	mcallw "github.com/viant/agently/pkg/agently/modelcall/write"
	payloadread "github.com/viant/agently/pkg/agently/payload/read"
	payloadw "github.com/viant/agently/pkg/agently/payload/write"
	toolw "github.com/viant/agently/pkg/agently/toolcall/write"
	turnw "github.com/viant/agently/pkg/agently/turn/write"
)

// Client is an in-memory implementation of conversation.Client.
// It is intended for tests and local runs without SQL/Datly.
type Client struct {
	mu            sync.RWMutex
	conversations map[string]*agconv.ConversationView
	// Indexes for fast lookup
	messages map[string]*agconv.MessageView
	payloads map[string]*payloadread.PayloadView
}

func New() *Client {
	return &Client{
		conversations: map[string]*agconv.ConversationView{},
		messages:      map[string]*agconv.MessageView{},
		payloads:      map[string]*payloadread.PayloadView{},
	}
}

// DeleteConversation removes a conversation and its dependent messages in-memory.
func (c *Client) DeleteConversation(_ context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// Remove messages belonging to the conversation
	for mid, mv := range c.messages {
		if mv != nil && mv.ConversationId == id {
			delete(c.messages, mid)
		}
	}
	// Remove conversation entry
	delete(c.conversations, id)
	return nil
}

// DeleteMessage removes a message by id from indexes and the conversation transcript.
func (c *Client) DeleteMessage(_ context.Context, conversationID, messageID string) error {
	if strings.TrimSpace(messageID) == "" {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.messages, messageID)
	if conv, ok := c.conversations[conversationID]; ok && conv != nil && conv.Transcript != nil {
		for _, t := range conv.Transcript {
			if t == nil || t.Message == nil {
				continue
			}
			kept := t.Message[:0]
			for _, m := range t.Message {
				if m == nil || m.Id == messageID {
					continue
				}
				kept = append(kept, m)
			}
			t.Message = kept
		}
	}
	return nil
}

// GetConversations returns all conversations without transcript for summary.
func (c *Client) GetConversations(ctx context.Context, input *convcli.Input) ([]*convcli.Conversation, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	// When a user is present in auth context, restrict listing to that user.
	// Otherwise, include all conversations.
	var userID string
	if ui := authctx.User(ctx); ui != nil {
		userID = strings.TrimSpace(ui.Subject)
		if userID == "" {
			userID = strings.TrimSpace(ui.Email)
		}
	}
	out := make([]*convcli.Conversation, 0, len(c.conversations))
	for _, v := range c.conversations {
		if v == nil {
			continue
		}
		if userID != "" {
			if v.CreatedByUserId == nil || strings.TrimSpace(*v.CreatedByUserId) != userID {
				continue
			}
		}
		cp := cloneConversationView(v)
		// Compute aggregated usage across entire conversation (not filtered)
		cp.Usage = c.aggregateUsage(v.Id)
		// Remove transcript for list view
		cp.Transcript = nil
		out = append(out, toClientConversation(cp))
	}
	// Stable order by CreatedAt asc then Id
	sort.Slice(out, func(i, j int) bool {
		if out[i] == nil || out[j] == nil {
			return false
		}
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].Id < out[j].Id
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

// GetConversation returns a single conversation with optional filtering.
func (c *Client) GetConversation(ctx context.Context, id string, options ...convcli.Option) (*convcli.Conversation, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	conv, ok := c.conversations[id]
	if !ok {
		return nil, nil
	}
	// When a user is present in auth context, only the owner may view.
	var userID string
	if ui := authctx.User(ctx); ui != nil {
		userID = strings.TrimSpace(ui.Subject)
		if userID == "" {
			userID = strings.TrimSpace(ui.Email)
		}
	}
	if userID != "" {
		if conv.CreatedByUserId == nil || strings.TrimSpace(*conv.CreatedByUserId) != userID {
			return nil, nil
		}
	}

	// Build input from options
	in := buildInput(id, options)

	// Clone first; aggregate usage against full conversation (not subject to since filter)
	cp := cloneConversationView(conv)
	cp.Usage = c.aggregateUsage(id)
	applySinceFilter(cp, &in)
	applyIncludeFlags(cp, &in)

	return toClientConversation(cp), nil
}

// aggregateUsage builds a UsageView equivalent to SQL aggregation for a conversation.
func (c *Client) aggregateUsage(conversationID string) *agconv.UsageView {
	if strings.TrimSpace(conversationID) == "" {
		return nil
	}
	// Accumulate totals and per-model stats
	type acc struct {
		pt, pct, pat, ct, crt, cat, capt, crpt, tt int
	}
	totals := acc{}
	byModel := map[string]*acc{}

	// Walk message index for matching conversation
	for _, m := range c.messages {
		if m == nil || m.ConversationId != conversationID || m.ModelCall == nil {
			continue
		}
		mc := m.ModelCall
		// Helper: fetch or create model accumulator
		get := func(model string) *acc {
			if v, ok := byModel[model]; ok {
				return v
			}
			v := &acc{}
			byModel[model] = v
			return v
		}
		mac := get(mc.Model)

		v := func(p *int) int {
			if p != nil {
				return *p
			}
			return 0
		}

		// Prompt
		x := v(mc.PromptTokens)
		totals.pt += x
		mac.pt += x
		x = v(mc.PromptCachedTokens)
		totals.pct += x
		mac.pct += x
		x = v(mc.PromptAudioTokens)
		totals.pat += x
		mac.pat += x

		// Completion
		x = v(mc.CompletionTokens)
		totals.ct += x
		mac.ct += x
		x = v(mc.CompletionReasoningTokens)
		totals.crt += x
		mac.crt += x
		x = v(mc.CompletionAudioTokens)
		totals.cat += x
		mac.cat += x
		x = v(mc.CompletionAcceptedPredictionTokens)
		totals.capt += x
		mac.capt += x
		x = v(mc.CompletionRejectedPredictionTokens)
		totals.crpt += x
		mac.crpt += x

		// Total
		x = v(mc.TotalTokens)
		totals.tt += x
		mac.tt += x
	}

	// No model calls → mirror SQL behavior (no row)
	if len(byModel) == 0 && totals.pt == 0 && totals.ct == 0 && totals.tt == 0 && totals.pct == 0 && totals.pat == 0 && totals.crt == 0 && totals.cat == 0 && totals.capt == 0 && totals.crpt == 0 {
		return nil
	}

	pint := func(i int) *int { v := i; return &v }

	// Build usage view with per-model breakdown (stable order by model)
	models := make([]*agconv.ModelView, 0, len(byModel))
	keys := make([]string, 0, len(byModel))
	for k := range byModel {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		a := byModel[k]
		models = append(models, &agconv.ModelView{
			ConversationId:                     conversationID,
			Model:                              k,
			PromptTokens:                       pint(a.pt),
			PromptCachedTokens:                 pint(a.pct),
			PromptAudioTokens:                  pint(a.pat),
			CompletionTokens:                   pint(a.ct),
			CompletionReasoningTokens:          pint(a.crt),
			CompletionAudioTokens:              pint(a.cat),
			CompletionAcceptedPredictionTokens: pint(a.capt),
			CompletionRejectedPredictionTokens: pint(a.crpt),
			TotalTokens:                        pint(a.tt),
		})
	}

	return &agconv.UsageView{
		ConversationId:                     conversationID,
		PromptTokens:                       pint(totals.pt),
		PromptCachedTokens:                 pint(totals.pct),
		PromptAudioTokens:                  pint(totals.pat),
		CompletionTokens:                   pint(totals.ct),
		CompletionReasoningTokens:          pint(totals.crt),
		CompletionAudioTokens:              pint(totals.cat),
		CompletionAcceptedPredictionTokens: pint(totals.capt),
		CompletionRejectedPredictionTokens: pint(totals.crpt),
		TotalTokens:                        pint(totals.tt),
		Model:                              models,
	}
}

// PatchConversations upserts conversations and merges fields according to Has flags.
func (c *Client) PatchConversations(ctx context.Context, in *convcli.MutableConversation) error {
	if in == nil || in.Has == nil || !in.Has.Id {
		return errors.New("missing conversation id")
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	cur, ok := c.conversations[in.Id]
	if !ok {
		// Create minimal conversation
		cur = &agconv.ConversationView{Id: in.Id, Stage: "", CreatedAt: time.Now()}
		// Default to private visibility and set owner when available
		cur.Visibility = "private"
		shareable := 0
		cur.Shareable = &shareable
		if ui := authctx.User(ctx); ui != nil {
			userID := strings.TrimSpace(ui.Subject)
			if userID == "" {
				userID = strings.TrimSpace(ui.Email)
			}
			if userID != "" {
				cur.CreatedByUserId = &userID
			}
		}
		c.conversations[in.Id] = cur
	}
	applyConversationPatch(cur, in)
	return nil
}

// GetPayload returns a payload by id.
func (c *Client) GetPayload(_ context.Context, id string) (*convcli.Payload, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	p, ok := c.payloads[id]
	if !ok {
		return nil, nil
	}
	cp := copyPayload((*convcli.Payload)(p))
	return cp, nil
}

// PatchPayload upserts a payload.
func (c *Client) PatchPayload(_ context.Context, p *convcli.MutablePayload) error {
	if p == nil || p.Has == nil || !p.Has.Id {
		return errors.New("missing payload id")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	existing, ok := c.payloads[p.Id]
	if !ok {
		c.payloads[p.Id] = &payloadread.PayloadView{Id: p.Id}
		existing = c.payloads[p.Id]
	}
	applyPayloadPatch((*convcli.Payload)(existing), p)
	return nil
}

// GetMessage returns a message by id.
func (c *Client) GetMessage(_ context.Context, id string, _ ...convcli.Option) (*convcli.Message, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	m, ok := c.messages[id]
	if !ok {
		return nil, nil
	}
	return toClientMessage(copyMessage(m)), nil
}

// GetMessageByElicitation returns a message matching the given conversation and elicitation IDs.
func (c *Client) GetMessageByElicitation(_ context.Context, conversationID, elicitationID string) (*convcli.Message, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	conv, ok := c.conversations[conversationID]
	if !ok || conv == nil || conv.Transcript == nil {
		return nil, nil
	}
	for _, t := range conv.Transcript {
		if t == nil || t.Message == nil {
			continue
		}
		for _, m := range t.Message {
			if m == nil || m.ElicitationId == nil {
				continue
			}
			if *m.ElicitationId == elicitationID {
				return toClientMessage(copyMessage(m)), nil
			}
		}
	}
	return nil, nil
}

// PatchMessage upserts a message and places it into its conversation/turn transcript.
func (c *Client) PatchMessage(_ context.Context, in *convcli.MutableMessage) error {
	if in == nil || in.Has == nil || !in.Has.Id || !in.Has.ConversationID {
		return errors.New("missing message id or conversation id")
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	conv, ok := c.conversations[in.ConversationID]
	if !ok {
		return errors.New("conversation not found")
	}

	// Ensure turn exists if provided
	var targetTurn *agconv.TranscriptView
	if in.Has.TurnID && in.TurnID != nil {
		targetTurn = findOrCreateTurn(conv, *in.TurnID)
	} else {
		// If no turn provided, attach to a default synthetic turn per conversation
		targetTurn = findOrCreateTurn(conv, defaultTurnID(conv.Id))
	}

	// Upsert message in index and in turn
	var cur *agconv.MessageView
	if existing, ok := c.messages[in.Id]; ok {
		cur = existing
	} else {
		cur = &agconv.MessageView{Id: in.Id, ConversationId: in.ConversationID, Role: in.Role, Type: in.Type, CreatedAt: time.Now()}
		c.messages[in.Id] = cur
		// Place into turn if not present
		if !messageInTurn(targetTurn, in.Id) {
			targetTurn.Message = append(targetTurn.Message, cur)
		}
	}
	applyMessagePatch(cur, in)
	return nil
}

// PatchModelCall upserts/attaches model-call to a message.
func (c *Client) PatchModelCall(_ context.Context, in *convcli.MutableModelCall) error {
	if in == nil || in.Has == nil || !in.Has.MessageID {
		return errors.New("missing model call message id")
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	msg, ok := c.messages[in.MessageID]
	if !ok {
		return errors.New("message not found")
	}
	if msg.ModelCall == nil {
		msg.ModelCall = &agconv.ModelCallView{MessageId: in.MessageID}
	}
	applyModelCallPatch(msg.ModelCall, in)
	return nil
}

// PatchToolCall upserts/attaches tool-call to a message.
func (c *Client) PatchToolCall(_ context.Context, in *convcli.MutableToolCall) error {
	if in == nil || in.Has == nil || !in.Has.MessageID || !in.Has.OpID {
		return errors.New("missing tool call identifiers")
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	msg, ok := c.messages[in.MessageID]
	if !ok {
		return errors.New("message not found")
	}
	if msg.ToolCall == nil {
		msg.ToolCall = &agconv.ToolCallView{MessageId: in.MessageID, OpId: in.OpID, Attempt: in.Attempt, ToolName: in.ToolName, ToolKind: in.ToolKind}
	}
	applyToolCallPatch(msg.ToolCall, in)
	return nil
}

// PatchTurn upserts a turn and attaches to a conversation.
func (c *Client) PatchTurn(_ context.Context, in *convcli.MutableTurn) error {
	if in == nil || in.Has == nil || !in.Has.Id || !in.Has.ConversationID {
		return errors.New("missing turn id or conversation id")
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	conv, ok := c.conversations[in.ConversationID]
	if !ok {
		return errors.New("conversation not found")
	}
	t := findOrCreateTurn(conv, in.Id)
	applyTurnPatch(t, in)
	return nil
}

// Helpers

// Internal helpers operating on agconv views

func buildInput(id string, options []convcli.Option) agconv.ConversationInput {
	in := agconv.ConversationInput{Id: id, Has: &agconv.ConversationInputHas{Id: true}}
	for _, opt := range options {
		if opt != nil {
			opt((*convcli.Input)(&in))
		}
	}
	return in
}

func cloneConversationView(src *agconv.ConversationView) *agconv.ConversationView {
	if src == nil {
		return nil
	}
	out := *src
	if src.Transcript != nil {
		out.Transcript = make([]*agconv.TranscriptView, 0, len(src.Transcript))
		for _, t := range src.Transcript {
			if t == nil {
				continue
			}
			tt := *t
			if t.Message != nil {
				tt.Message = make([]*agconv.MessageView, 0, len(t.Message))
				for _, m := range t.Message {
					tt.Message = append(tt.Message, copyMessage(m))
				}
			}
			out.Transcript = append(out.Transcript, &tt)
		}
	}
	return &out
}

func copyMessage(m *agconv.MessageView) *agconv.MessageView {
	if m == nil {
		return nil
	}
	cp := *m
	if m.Attachment != nil {
		cp.Attachment = make([]*agconv.AttachmentView, len(m.Attachment))
		copy(cp.Attachment, m.Attachment)
	}
	if m.ModelCall != nil {
		tmp := *m.ModelCall
		cp.ModelCall = &tmp
	}
	if m.ToolCall != nil {
		tmp := *m.ToolCall
		cp.ToolCall = &tmp
	}
	return &cp
}

func copyPayload(p *convcli.Payload) *convcli.Payload {
	if p == nil {
		return nil
	}
	cp := *p
	if p.InlineBody != nil {
		b := make([]byte, len(*p.InlineBody))
		copy(b, *p.InlineBody)
		cp.InlineBody = &b
	}
	return &cp
}

func findOrCreateTurn(conv *agconv.ConversationView, turnID string) *agconv.TranscriptView {
	if conv.Transcript == nil {
		conv.Transcript = []*agconv.TranscriptView{}
	}
	for _, t := range conv.Transcript {
		if t != nil && t.Id == turnID {
			return t
		}
	}
	t := &agconv.TranscriptView{Id: turnID, ConversationId: conv.Id, Status: "active", CreatedAt: time.Now()}
	conv.Transcript = append(conv.Transcript, t)
	sort.SliceStable(conv.Transcript, func(i, j int) bool { return conv.Transcript[i].CreatedAt.Before(conv.Transcript[j].CreatedAt) })
	return t
}

func messageInTurn(t *agconv.TranscriptView, id string) bool {
	for _, m := range t.Message {
		if m != nil && m.Id == id {
			return true
		}
	}
	return false
}

func toClientConversation(v *agconv.ConversationView) *convcli.Conversation {
	if v == nil {
		return nil
	}
	c := convcli.Conversation(*v)
	return &c
}

func toClientMessage(v *agconv.MessageView) *convcli.Message {
	if v == nil {
		return nil
	}
	m := convcli.Message(*v)
	return &m
}

func applySinceFilter(conv *agconv.ConversationView, in *agconv.ConversationInput) {
	if conv == nil || in == nil || in.Has == nil || !in.Has.Since || strings.TrimSpace(in.Since) == "" || conv.Transcript == nil {
		return
	}
	turnID := in.Since
	var sinceTime *time.Time
	for _, t := range conv.Transcript {
		if t != nil && t.Id == turnID {
			ts := t.CreatedAt
			sinceTime = &ts
			break
		}
	}
	if sinceTime == nil {
		return
	}
	filtered := make([]*agconv.TranscriptView, 0, len(conv.Transcript))
	for _, t := range conv.Transcript {
		if t != nil && (t.CreatedAt.Equal(*sinceTime) || t.CreatedAt.After(*sinceTime)) {
			filtered = append(filtered, t)
		}
	}
	conv.Transcript = filtered
}

func applyIncludeFlags(conv *agconv.ConversationView, in *agconv.ConversationInput) {
	if conv == nil || conv.Transcript == nil {
		return
	}
	includeModel := in != nil && in.Has != nil && in.Has.IncludeModelCal && in.IncludeModelCal
	includeTool := in != nil && in.Has != nil && in.Has.IncludeToolCall && in.IncludeToolCall
	if includeModel && includeTool {
		return
	}
	for _, t := range conv.Transcript {
		for _, m := range t.Message {
			if !includeModel {
				m.ModelCall = nil
			}
			if !includeTool {
				m.ToolCall = nil
			}
		}
	}
}

func defaultTurnID(convID string) string { return convID + ":turn" }

// Patch appliers

func applyConversationPatch(dst *agconv.ConversationView, src *convcli.MutableConversation) {
	if src.Has == nil {
		return
	}
	if src.Has.Summary {
		dst.Summary = src.Summary
	}
	if src.Has.AgentId {
		dst.AgentId = &src.AgentId
	}
	if src.Has.ConversationParentId {
		dst.ConversationParentId = &src.ConversationParentId
	}
	if src.Has.ConversationParentTurnId {
		dst.ConversationParentTurnId = &src.ConversationParentTurnId
	}
	if src.Has.Title {
		dst.Title = src.Title
	}
	if src.Has.Visibility && src.Visibility != nil {
		dst.Visibility = *src.Visibility
	} // view has non-pointer Visibility
	if src.Has.Shareable {
		dst.Shareable = &src.Shareable
	}
	if src.Has.CreatedAt && src.CreatedAt != nil {
		dst.CreatedAt = *src.CreatedAt
	}
	if src.Has.LastActivity && src.LastActivity != nil {
		dst.LastActivity = src.LastActivity
	}
	if src.Has.UsageInputTokens {
		dst.UsageInputTokens = &src.UsageInputTokens
	}
	if src.Has.UsageOutputTokens {
		dst.UsageOutputTokens = &src.UsageOutputTokens
	}
	if src.Has.UsageEmbeddingTokens {
		dst.UsageEmbeddingTokens = &src.UsageEmbeddingTokens
	}
	if src.Has.CreatedByUserID {
		dst.CreatedByUserId = src.CreatedByUserID
	}
	if src.Has.DefaultModelProvider {
		dst.DefaultModelProvider = src.DefaultModelProvider
	}
	if src.Has.DefaultModel {
		dst.DefaultModel = src.DefaultModel
	}
	if src.Has.DefaultModelParams {
		dst.DefaultModelParams = src.DefaultModelParams
	}
	if src.Has.Metadata {
		dst.Metadata = src.Metadata
	}
}

func applyMessagePatch(dst *agconv.MessageView, src *msgw.Message) {
	if src.Has == nil {
		return
	}
	if src.Has.ConversationID {
		dst.ConversationId = src.ConversationID
	}
	if src.Has.TurnID {
		dst.TurnId = src.TurnID
	}
	if src.Has.Sequence {
		dst.Sequence = src.Sequence
	}
	if src.Has.CreatedAt && src.CreatedAt != nil {
		dst.CreatedAt = *src.CreatedAt
	}
	if src.Has.CreatedByUserID {
		dst.CreatedByUserId = src.CreatedByUserID
	}
	if src.Has.Role {
		dst.Role = src.Role
	}
	if src.Has.Status {
		dst.Status = &src.Status
	}
	if src.Has.Type {
		dst.Type = src.Type
	}
	if src.Has.Content {
		if src.Content == "" {
			dst.Content = nil
		} else {
			s := src.Content
			dst.Content = &s
		}
	}
	if src.Has.RawContent {
		if src.RawContent == nil || *src.RawContent == "" {
			dst.RawContent = nil
		} else {
			val := *src.RawContent
			dst.RawContent = &val
		}
	}
	if src.Has.ContextSummary {
		dst.ContextSummary = src.ContextSummary
	}
	if src.Has.Tags {
		dst.Tags = src.Tags
	}
	if src.Has.Interim {
		if src.Interim != nil {
			dst.Interim = *src.Interim
		}
	}
	if src.Has.ElicitationID {
		dst.ElicitationId = src.ElicitationID
	}
	if src.Has.ParentMessageID {
		dst.ParentMessageId = src.ParentMessageID
	}
	if src.Has.SupersededBy {
		dst.SupersededBy = src.SupersededBy
	}
	if src.Has.ToolName {
		dst.ToolName = src.ToolName
	}
	if src.Has.AttachmentPayloadID {
		dst.AttachmentPayloadId = src.AttachmentPayloadID
	}
	if src.Has.ElicitationPayloadID {
		dst.ElicitationPayloadId = src.ElicitationPayloadID
	}
}

func applyModelCallPatch(dst *agconv.ModelCallView, src *mcallw.ModelCall) {
	if src.Has == nil {
		return
	}
	if src.Has.TurnID {
		dst.TurnId = src.TurnID
	}
	if src.Has.Provider {
		dst.Provider = src.Provider
	}
	if src.Has.Model {
		dst.Model = src.Model
	}
	if src.Has.ModelKind {
		dst.ModelKind = src.ModelKind
	}
	if src.Has.Status {
		dst.Status = src.Status
	}
	if src.Has.ErrorCode {
		dst.ErrorCode = src.ErrorCode
	}
	if src.Has.ErrorMessage {
		dst.ErrorMessage = src.ErrorMessage
	}
	if src.Has.PromptTokens {
		dst.PromptTokens = src.PromptTokens
	}
	if src.Has.PromptCachedTokens {
		dst.PromptCachedTokens = src.PromptCachedTokens
	}
	if src.Has.CompletionTokens {
		dst.CompletionTokens = src.CompletionTokens
	}
	if src.Has.TotalTokens {
		dst.TotalTokens = src.TotalTokens
	}
	if src.Has.StartedAt {
		dst.StartedAt = src.StartedAt
	}
	if src.Has.CompletedAt {
		dst.CompletedAt = src.CompletedAt
	}
	if src.Has.LatencyMS {
		dst.LatencyMs = src.LatencyMS
	}
	if src.Has.Cost {
		dst.Cost = src.Cost
	}
	if src.Has.TraceID {
		dst.TraceId = src.TraceID
	}
	if src.Has.SpanID {
		dst.SpanId = src.SpanID
	}
	if src.Has.RequestPayloadID {
		dst.RequestPayloadId = src.RequestPayloadID
	}
	if src.Has.ResponsePayloadID {
		dst.ResponsePayloadId = src.ResponsePayloadID
	}
	if src.Has.ProviderRequestPayloadID {
		dst.ProviderRequestPayloadId = src.ProviderRequestPayloadID
	}
	if src.Has.ProviderResponsePayloadID {
		dst.ProviderResponsePayloadId = src.ProviderResponsePayloadID
	}
	if src.Has.StreamPayloadID {
		dst.StreamPayloadId = src.StreamPayloadID
	}
}

func applyToolCallPatch(dst *agconv.ToolCallView, src *toolw.ToolCall) {
	if src.Has == nil {
		return
	}
	if src.Has.TurnID {
		dst.TurnId = src.TurnID
	}
	if src.Has.OpID {
		dst.OpId = src.OpID
	}
	if src.Has.Attempt {
		dst.Attempt = src.Attempt
	}
	if src.Has.ToolName {
		dst.ToolName = src.ToolName
	}
	if src.Has.ToolKind {
		dst.ToolKind = src.ToolKind
	}
	// CapabilityTags and ResourceURIs removed
	if src.Has.Status {
		dst.Status = src.Status
	}
	// RequestSnapshot removed
	if src.Has.RequestHash {
		dst.RequestHash = src.RequestHash
	}
	// ResponseSnapshot removed
	if src.Has.ErrorCode {
		dst.ErrorCode = src.ErrorCode
	}
	if src.Has.ErrorMessage {
		dst.ErrorMessage = src.ErrorMessage
	}
	if src.Has.Retriable {
		dst.Retriable = src.Retriable
	}
	if src.Has.StartedAt {
		dst.StartedAt = src.StartedAt
	}
	if src.Has.CompletedAt {
		dst.CompletedAt = src.CompletedAt
	}
	if src.Has.LatencyMS {
		dst.LatencyMs = src.LatencyMS
	}
	if src.Has.Cost {
		dst.Cost = src.Cost
	}
	if src.Has.TraceID {
		dst.TraceId = src.TraceID
	}
	if src.Has.SpanID {
		dst.SpanId = src.SpanID
	}
	if src.Has.RequestPayloadID {
		dst.RequestPayloadId = src.RequestPayloadID
	}
	if src.Has.ResponsePayloadID {
		dst.ResponsePayloadId = src.ResponsePayloadID
	}
}

func applyTurnPatch(dst *agconv.TranscriptView, src *turnw.Turn) {
	if src.Has == nil {
		return
	}
	if src.Has.ConversationID {
		dst.ConversationId = src.ConversationID
	}
	if src.Has.CreatedAt && src.CreatedAt != nil {
		dst.CreatedAt = *src.CreatedAt
	}
	if src.Has.Status {
		dst.Status = src.Status
	}
	if src.Has.StartedByMessageID {
		dst.StartedByMessageId = src.StartedByMessageID
	}
	if src.Has.RetryOf {
		dst.RetryOf = src.RetryOf
	}
	if src.Has.AgentIDUsed {
		dst.AgentIdUsed = src.AgentIDUsed
	}
	if src.Has.AgentConfigUsedID {
		dst.AgentConfigUsedId = src.AgentConfigUsedID
	}
	if src.Has.ModelOverrideProvider {
		dst.ModelOverrideProvider = src.ModelOverrideProvider
	}
	if src.Has.ModelOverride {
		dst.ModelOverride = src.ModelOverride
	}
	if src.Has.ModelParamsOverride {
		dst.ModelParamsOverride = src.ModelParamsOverride
	}
}

func applyPayloadPatch(dst *convcli.Payload, src *payloadw.Payload) {
	if src.Has == nil {
		return
	}
	if src.Has.TenantID {
		dst.TenantID = src.TenantID
	}
	if src.Has.Kind {
		dst.Kind = src.Kind
	}
	if src.Has.Subtype {
		dst.Subtype = src.Subtype
	}
	if src.Has.MimeType {
		dst.MimeType = src.MimeType
	}
	if src.Has.SizeBytes {
		dst.SizeBytes = src.SizeBytes
	}
	if src.Has.Digest {
		dst.Digest = src.Digest
	}
	if src.Has.Storage {
		dst.Storage = src.Storage
	}
	if src.Has.InlineBody {
		dst.InlineBody = (*[]byte)(src.InlineBody)
	}
	if src.Has.URI {
		dst.URI = src.URI
	}
	if src.Has.Compression {
		dst.Compression = src.Compression
	}
	if src.Has.EncryptionKMSKeyID {
		dst.EncryptionKMSKeyID = src.EncryptionKMSKeyID
	}
	if src.Has.RedactionPolicyVersion {
		dst.RedactionPolicyVersion = src.RedactionPolicyVersion
	}
	if src.Has.Redacted {
		dst.Redacted = src.Redacted
	}
	if src.Has.CreatedAt {
		dst.CreatedAt = src.CreatedAt
	}
	if src.Has.SchemaRef {
		dst.SchemaRef = src.SchemaRef
	}
}

// Bootstrap helpers used by tests

// EnsureConversation creates a conversation if it does not exist.
func (c *Client) EnsureConversation(id string, opts ...func(*convcli.MutableConversation)) error {
	mc := convcli.NewConversation()
	mc.SetId(id)
	for _, o := range opts {
		if o != nil {
			o(mc)
		}
	}
	return c.PatchConversations(context.Background(), mc)
}
