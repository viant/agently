package core

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/llm/provider/base"
	"github.com/viant/agently/genai/memory"
	modelcallctx "github.com/viant/agently/genai/modelcallctx"
	"github.com/viant/agently/genai/prompt"
	svc "github.com/viant/agently/genai/tool/service"
)

type GenerateInput struct {
	llm.ModelSelection
	SystemPrompt *prompt.Prompt

	Prompt  *prompt.Prompt
	Binding *prompt.Binding
	Message []llm.Message
	// ExpandedUserPrompt holds the fully expanded user task text
	// produced from the user template and binding. Callers that
	// wish to persist the expanded task as the canonical user
	// message (instead of the raw input) can read this value
	// after Init completes.
	ExpandedUserPrompt string `yaml:"expandedUserPrompt,omitempty" json:"expandedUserPrompt,omitempty"`

	// UserPromptAlreadyInHistory, when true, signals that the caller has
	// already persisted the expanded user prompt as the latest user
	// message in Binding.History.Past. In that case, Init will not
	// append a synthetic chat_user message into History.Current. When
	// false (default), Init may add a synthetic user message for the
	// current turn.
	UserPromptAlreadyInHistory bool `yaml:"userPromptAlreadyInHistory,omitempty" json:"userPromptAlreadyInHistory,omitempty"`

	// IncludeCurrentHistory controls whether History.Current (in-flight
	// turn messages) should be included when building the LLM request
	// messages. When false, only Past (committed) turns participate;
	// when true (default), Past then Current are flattened.
	IncludeCurrentHistory bool `yaml:"includeCurrentHistory,omitempty" json:"includeCurrentHistory,omitempty"`
	// Participant identities for multi-user/agent attribution
	UserID  string `yaml:"userID" json:"userID"`
	AgentID string `yaml:"agentID" json:"agentID"`
}

// GenerateOutput represents output from extraction
type GenerateOutput struct {
	Response  *llm.GenerateResponse
	Content   string
	MessageID string
}

// ExpandUserPromptInput represents a lightweight request to expand only the
// user prompt template given a binding, without constructing a full
// GenerateRequest or calling the model. It mirrors the user-facing portion
// of GenerateInput.
type ExpandUserPromptInput struct {
	Prompt  *prompt.Prompt  `json:"prompt,omitempty"`
	Binding *prompt.Binding `json:"binding,omitempty"`
}

// ExpandUserPromptOutput carries the expanded user prompt text.
type ExpandUserPromptOutput struct {
	ExpandedUserPrompt string `json:"expandedUserPrompt"`
}

func (i *GenerateInput) MatchModelIfNeeded(matcher llm.Matcher) {
	if i.Model != "" || i.Preferences == nil {
		return
	}
	// When gatekeeper filters are set on selection, reduce candidates first if supported.
	if rm, ok := matcher.(llm.ReducingMatcher); ok && (len(i.AllowedModels) > 0 || len(i.AllowedProviders) > 0) {
		allowSet := map[string]struct{}{}
		for _, m := range i.AllowedModels {
			if v := strings.TrimSpace(m); v != "" {
				allowSet[v] = struct{}{}
			}
		}
		provSet := map[string]struct{}{}
		for _, p := range i.AllowedProviders {
			if v := strings.TrimSpace(p); v != "" {
				provSet[v] = struct{}{}
			}
		}
		allow := func(id string) bool {
			id = strings.TrimSpace(strings.ToLower(id))
			if id == "" {
				return false
			}
			if len(allowSet) > 0 {
				_, ok := allowSet[id]
				return ok
			}
			if len(provSet) > 0 {
				if idx := strings.IndexByte(id, '_'); idx > 0 {
					_, ok := provSet[id[:idx]]
					return ok
				}
				return false
			}
			return true
		}
		if m := rm.BestWithFilter(i.Preferences, allow); m != "" {
			i.Model = m
			return
		}
	}
	if m := matcher.Best(i.Preferences); m != "" {
		i.Model = m
	}
}

func (i *GenerateInput) Init(ctx context.Context) error {

	if i.SystemPrompt != nil {
		if err := i.SystemPrompt.Init(ctx); err != nil {
			return err
		}
		expanded, err := i.SystemPrompt.Generate(ctx, i.Binding.SystemBinding())
		if err != nil {
			return fmt.Errorf("failed to expand system prompt: %w", err)
		}
		i.Message = append(i.Message, llm.NewSystemMessage(expanded))
	}

	// Note: attachments are appended in two places:
	// - from conversation history (persisted attachments) below
	// - from the current task binding (ad-hoc attachments) before the user message

	if i.Prompt == nil {
		i.Prompt = &prompt.Prompt{}
	}
	if err := i.Prompt.Init(ctx); err != nil {
		return err
	}
	currentPrompt, err := i.Prompt.Generate(ctx, i.Binding)
	if err != nil {
		return fmt.Errorf("failed to prompt: %w", err)
	}
	// Expose the expanded user prompt so callers that manage
	// their own conversation messages can reuse it.
	i.ExpandedUserPrompt = currentPrompt

	// Record current user task into History.Current so it participates in
	// a unified, chronological view of the in-flight turn.
	if i.Binding != nil {
		// If the caller has already persisted the expanded user prompt
		// as the latest user message in History.Past, do not append a
		// synthetic user message into History.Current.
		if !i.UserPromptAlreadyInHistory {
			// When the last persisted user message already matches the
			// expanded prompt, avoid appending a duplicate synthetic user
			// message. This allows callers that have already written the
			// expanded task into the conversation to keep history clean.
			shouldAppend := true
			trimmed := strings.TrimSpace(currentPrompt)
			if trimmed != "" && len(i.Binding.History.Past) > 0 {
				h := &i.Binding.History
				for ti := len(h.Past) - 1; ti >= 0 && shouldAppend; ti-- {
					turn := h.Past[ti]
					if turn == nil || len(turn.Messages) == 0 {
						continue
					}
					for mi := len(turn.Messages) - 1; mi >= 0; mi-- {
						m := turn.Messages[mi]
						if m == nil {
							continue
						}
						if !strings.EqualFold(strings.TrimSpace(m.Role), "user") {
							continue
						}
						if strings.TrimSpace(m.Content) == trimmed {
							shouldAppend = false
							break
						}
					}
				}
			}
			if shouldAppend {
				msg := &prompt.Message{
					Kind:    prompt.MessageKindChatUser,
					Role:    string(llm.RoleUser),
					Content: currentPrompt,
				}
				// Reuse task-scoped attachments for the current user message.
				if len(i.Binding.Task.Attachments) > 0 {
					sortAttachments(i.Binding.Task.Attachments)
					msg.Attachment = i.Binding.Task.Attachments
				}
				appendCurrentHistoryMessages(&i.Binding.History, msg)
			}
		}
	}

	if i.Binding != nil {
		for _, doc := range i.Binding.SystemDocuments.Items {
			i.Message = append(i.Message, llm.NewTextMessage(llm.MessageRole("system"), doc.PageContent))
		}
	}

	// TODO change place - before history when full documents are used, after history when snippets are used
	if i.Binding != nil {
		for _, doc := range i.Binding.Documents.Items {
			i.Message = append(i.Message, llm.NewTextMessage(llm.MessageRole("user"), doc.PageContent))
		}
	}

	if i.Binding != nil {
		msgs := i.Binding.History.LLMMessages()
		// When IncludeCurrentHistory is false, callers expect only
		// committed Past turns to participate in the prompt. In that
		// case, LLMMessages will not distinguish, so we rely on the
		// caller to have left History.Current nil or empty. The
		// default path includes both Past and Current.
		if !i.IncludeCurrentHistory && i.Binding.History.Current != nil {
			// Filter out any messages that originated from the in-flight
			// Current turn by excluding those whose ids match
			// History.Current messages.
			currentIDs := map[string]struct{}{}
			for _, cm := range i.Binding.History.Current.Messages {
				if cm == nil {
					continue
				}
				if id := strings.TrimSpace(cm.ID); id != "" {
					currentIDs[id] = struct{}{}
				}
			}
			filtered := make([]llm.Message, 0, len(msgs))
			for _, m := range msgs {
				// There is no ID on llm.Message, so we cannot reliably
				// filter by id here without changing llm.Message. Callers
				// that require strict Past-only behavior should avoid
				// populating History.Current.
				filtered = append(filtered, m)
			}
			msgs = filtered
		}
		for _, msg := range msgs {
			i.Message = append(i.Message, msg)
		}
	}

	if tools := i.Binding.Tools; len(tools.Signatures) > 0 {
		for _, tool := range tools.Signatures {
			i.Options.Tools = append(i.Options.Tools, llm.Tool{Type: "function", Ref: "", Definition: *tool})
		}
	}

	if i.Binding.History.Current != nil {
		for _, elicitationMsg := range i.Binding.History.Current.Messages {
			if elicitationMsg == nil {
				continue
			}
			// Only append elicitation messages of the current turn.
			if elicitationMsg.Kind != prompt.MessageKindElicitPrompt && elicitationMsg.Kind != prompt.MessageKindElicitAnswer {
				continue
			}
			i.Message = append(i.Message, llm.NewTextMessage(llm.MessageRole(elicitationMsg.Role), elicitationMsg.Content))
			// Debug: keys or a short sample (unchanged)
			content := strings.TrimSpace(elicitationMsg.Content)
			keys := []string{}
			if content != "" && strings.HasPrefix(content, "{") {
				var tmp map[string]interface{}
				if err := json.Unmarshal([]byte(content), &tmp); err == nil {
					for k := range tmp {
						keys = append(keys, k)
					}
					sort.Strings(keys)
				}
			}
		}
	}

	return nil
}

func sortAttachments(attachments []*prompt.Attachment) {
	sort.Slice(attachments, func(i, j int) bool {
		if attachments[i] == nil || attachments[j] == nil {
			return false
		}
		if strings.Compare(attachments[i].URI, attachments[j].URI) < 0 {
			return true
		}
		return false
	})
}

// appendCurrentHistoryMessages appends messages to History.Current ensuring
// CreatedAt is set and non-decreasing within the current turn.
func appendCurrentHistoryMessages(h *prompt.History, msgs ...*prompt.Message) {
	if h == nil || len(msgs) == 0 {
		return
	}
	if h.Current == nil {
		h.Current = &prompt.Turn{ID: h.CurrentTurnID}
	}
	for _, m := range msgs {
		if m == nil {
			continue
		}
		if m.CreatedAt.IsZero() {
			m.CreatedAt = time.Now().UTC()
		}
		if n := len(h.Current.Messages); n > 0 {
			last := h.Current.Messages[n-1].CreatedAt
			if m.CreatedAt.Before(last) {
				m.CreatedAt = last.Add(time.Nanosecond)
			}
		}
		h.Current.Messages = append(h.Current.Messages, m)
	}
}

func (i *GenerateInput) Validate(ctx context.Context) error {
	if strings.TrimSpace(i.UserID) == "" {
		return fmt.Errorf("userId is required")
	}
	if i.Model == "" {
		return fmt.Errorf("model is required")
	}
	if len(i.Message) == 0 {
		return fmt.Errorf("content is required")
	}
	return nil
}

// generate processes LLM responses to generate structured data
func (s *Service) generate(ctx context.Context, in, out interface{}) error {
	input, ok := in.(*GenerateInput)
	if !ok {
		return svc.NewInvalidInputError(in)
	}
	output, ok := out.(*GenerateOutput)
	if !ok {
		return svc.NewInvalidOutputError(out)
	}

	return s.Generate(ctx, input, output)
}

// expandUserPrompt expands only the user prompt template for the provided
// binding and returns the resulting text without invoking any model call.
// It is intended for callers (e.g., dev_coder or orchestrators) that wish
// to persist the expanded task as the canonical user message before a full
// generate invocation.
func (s *Service) expandUserPrompt(ctx context.Context, in, out interface{}) error {
	input, ok := in.(*ExpandUserPromptInput)
	if !ok {
		return svc.NewInvalidInputError(in)
	}
	output, ok := out.(*ExpandUserPromptOutput)
	if !ok {
		return svc.NewInvalidOutputError(out)
	}

	// Default prompt when none provided
	p := input.Prompt
	if p == nil {
		p = &prompt.Prompt{}
	}
	if err := p.Init(ctx); err != nil {
		return fmt.Errorf("failed to init prompt: %w", err)
	}
	// Ensure binding is non-nil so templates have a stable binding.
	if input.Binding == nil {
		input.Binding = &prompt.Binding{}
	}
	expanded, err := p.Generate(ctx, input.Binding)
	if err != nil {
		return fmt.Errorf("failed to expand user prompt: %w", err)
	}
	output.ExpandedUserPrompt = expanded
	return nil
}

func (s *Service) Generate(ctx context.Context, input *GenerateInput, output *GenerateOutput) error {

	// Inject recorder observer with price resolver (if available) so per-call cost is computed.
	if tp, ok := s.llmFinder.(modelcallctx.TokenPriceProvider); ok {
		declared := strings.TrimSpace(input.Model)
		if declared != "" {
			tp = modelcallctx.NewFixedModelPriceProvider(tp, declared)
		}
		ctx = modelcallctx.WithRecorderObserverWithPrice(ctx, s.convClient, tp)
	} else {
		ctx = modelcallctx.WithRecorderObserver(ctx, s.convClient)
	}
	request, model, err := s.prepareGenerateRequest(ctx, input)
	if err != nil {
		return err
	}
	// Debug: summarize messages with tool calls and tool_call_id prior to generate
	var withCalls, withCallID int
	for _, m := range request.Messages {
		if len(m.ToolCalls) > 0 {
			withCalls += len(m.ToolCalls)
		}
		if strings.TrimSpace(m.ToolCallId) != "" {
			withCallID++
		}
	}
	// Handle continuation-by-anchor in a dedicated helper for clarity.
	if lr, handled, cerr := s.tryGenerateContinuationByAnchor(ctx, model, request); handled || cerr != nil {
		if cerr != nil {
			return cerr
		}
		output.Response = lr
		if lr != nil {
			var builder strings.Builder
			for _, choice := range lr.Choices {
				if len(choice.Message.ToolCalls) > 0 {
					continue
				}
				if txt := strings.TrimSpace(choice.Message.Content); txt != "" {
					builder.WriteString(txt)
					continue
				}
				for _, item := range choice.Message.Items {
					if item.Type != llm.ContentTypeText {
						continue
					}
					if item.Data != "" {
						builder.WriteString(item.Data)
					} else if item.Text != "" {
						builder.WriteString(item.Text)
					}
				}
			}
			output.Content = strings.TrimSpace(builder.String())
			if msgID := memory.ModelMessageIDFromContext(ctx); msgID != "" {
				output.MessageID = msgID
			}
		}
		return nil
	}

	// Attach finish barrier to upstream ctx so recorder observer can signal completion (payload ids, usage).
	ctx, _ = modelcallctx.WithFinishBarrier(ctx)
	if s.streamPub != nil {
		if input == nil || input.Options == nil || strings.ToLower(strings.TrimSpace(input.Options.Mode)) != "plan" {
			ctx = modelcallctx.WithStreamPublisher(ctx, s.streamPub)
		}
	}
	// Retry transient connectivity/network errors up to 3 attempts with
	// 1s initial delay and exponential backoff (1s, 2s, 4s). Additionally,
	// consult provider-specific backoff advisor when available (e.g., Bedrock
	// ThrottlingException -> 30s wait) before the next attempt.
	var response *llm.GenerateResponse
	for attempt := 0; attempt < 3; attempt++ {
		response, err = model.Generate(ctx, request)
		if err == nil {
			break
		}
		// Do not retry on provider/model context-limit errors; surface a sentinel error
		if isContextLimitError(err) {
			return fmt.Errorf("%w: %v", ErrContextLimitExceeded, err)
		}
		// Provider-specific backoff advice (optional)
		if advisor, ok := model.(llm.BackoffAdvisor); ok {
			if delay, retry := advisor.AdviseBackoff(err, attempt); retry {
				if attempt == 2 || ctx.Err() != nil {
					return fmt.Errorf("failed to generate content: %w", err)
				}
				// Set model_call status to retrying before waiting
				s.setModelCallStatus(ctx, "retrying")
				select {
				case <-time.After(delay):
				case <-ctx.Done():
					return fmt.Errorf("failed to generate content: %w", err)
				}
				continue
			}
		}
		if !isTransientNetworkError(err) || attempt == 2 || ctx.Err() != nil {
			return fmt.Errorf("failed to generate content: %w", err)
		}
		// 1s, 2s, 4s backoff
		delay := time.Second << attempt
		// Set model_call status to retrying before waiting
		s.setModelCallStatus(ctx, "retrying")
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return fmt.Errorf("failed to generate content: %w", err)
		}
	}
	output.Response = response
	if response != nil {
		// Log response id and any tool calls emitted
		total := 0
		var ids []string
		for _, choice := range response.Choices {
			if len(choice.Message.ToolCalls) == 0 {
				continue
			}
			total += len(choice.Message.ToolCalls)
			for _, tc := range choice.Message.ToolCalls {
				ids = append(ids, strings.TrimSpace(tc.ID)+":"+strings.TrimSpace(tc.Name))
			}
		}
	}

	// Usage aggregation is now handled by provider-level UsageListener attached
	// in the model finder. Avoid double-counting here.
	var builder strings.Builder
	for _, choice := range response.Choices {
		if len(choice.Message.ToolCalls) > 0 {
			continue
		}
		if txt := strings.TrimSpace(choice.Message.Content); txt != "" {
			builder.WriteString(txt)
			continue // prefer Content when provided, avoid double printing
		}

		for _, item := range choice.Message.Items {
			if item.Type != llm.ContentTypeText {
				continue
			}
			if item.Data != "" {
				builder.WriteString(item.Data)
			} else if item.Text != "" {
				builder.WriteString(item.Text)
			}
		}
	}

	output.Content = strings.TrimSpace(builder.String())

	// Transient debug: if more than one tool call was emitted in a single
	// provider response, print a concise trace line to aid troubleshooting.
	// This is intentionally unconditional (no env gate) and low volume.
	// It does not leak credentials and includes conversation/turn ids.
	totalToolCalls := 0
	var toolNames []string
	for _, choice := range response.Choices {
		if len(choice.Message.ToolCalls) == 0 {
			continue
		}
		totalToolCalls += len(choice.Message.ToolCalls)
		for _, tc := range choice.Message.ToolCalls {
			name := strings.TrimSpace(tc.Name)
			if name == "" {
				name = "(unnamed)"
			}
			toolNames = append(toolNames, name)
		}
	}
	// Provide the shared assistant message ID to the caller; orchestrator writes the final assistant message.
	if msgID := memory.ModelMessageIDFromContext(ctx); msgID != "" {
		output.MessageID = msgID
	}
	return nil
}

// ErrContextLimitExceeded signals that a provider/model rejected the request due to
// exceeding the maximum context window (prompt too long / too many tokens).
var ErrContextLimitExceeded = errors.New("llm/core: context limit exceeded")

// ContinuationContextLimitError marks context-limit failures during continuation (previous_response_id) calls.
// It unwraps to ErrContextLimitExceeded so existing recovery flows still trigger.
type ContinuationContextLimitError struct {
	Err error
}

func (e ContinuationContextLimitError) Error() string {
	return fmt.Sprintf("llm/core: continuation context limit exceeded: %v", e.Err)
}

func (e ContinuationContextLimitError) Unwrap() error { return ErrContextLimitExceeded }

// IsContinuationContextLimit reports whether the error is a continuation context-limit failure.
func IsContinuationContextLimit(err error) bool {
	var e ContinuationContextLimitError
	return errors.As(err, &e)
}

// isContextLimitError heuristically classifies provider/model errors indicating
// the prompt/context exceeded the model's maximum capacity.
func isContextLimitError(err error) bool {
	if err == nil {
		return false
	}
	// Unwrap and inspect message text; providers vary widely in phrasing.
	msg := strings.ToLower(err.Error())
	return ContainsContextLimitError(msg)
}

func ContainsContextLimitError(input string) bool {
	switch {
	case strings.Contains(input, "context length exceeded"),
		strings.Contains(input, "maximum context length"),
		strings.Contains(input, "exceeds context length"),
		strings.Contains(input, "exceeds the context window"),
		strings.Contains(input, "context window is") && strings.Contains(input, "exceeded"),
		strings.Contains(input, "prompt is too long"),
		strings.Contains(input, "prompt too long"),
		strings.Contains(input, "token limit"),
		strings.Contains(input, "too many tokens"),
		strings.Contains(input, "input is too long"),
		strings.Contains(input, "request too large"),
		strings.Contains(input, "context_length_exceeded"), // common provider code
		strings.Contains(input, "resourceexhausted") && strings.Contains(input, "context"):
		return true
	case strings.Contains(input, "request too large"):
		return true
	}
	return false
}

// isTransientNetworkError heuristically classifies errors that are likely
// transient connectivity/network failures worth retrying.
func isTransientNetworkError(err error) bool {
	if err == nil {
		return false
	}
	// net.Error with Timeout/Temporary
	var nerr net.Error
	if errors.As(err, &nerr) {
		if nerr.Timeout() {
			return true
		}
		// Temporary is deprecated but still useful when implemented
		type temporary interface{ Temporary() bool }
		if t, ok := any(nerr).(temporary); ok && t.Temporary() {
			return true
		}
	}
	// Context deadline exceeded is often a transient provider/backbone failure
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	// String heuristics for common transient failures
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "connection reset"),
		strings.Contains(msg, "connection refused"),
		strings.Contains(msg, "dial tcp"),
		strings.Contains(msg, "i/o timeout"),
		strings.Contains(msg, "tls handshake"),
		strings.Contains(msg, "temporary network error"),
		strings.Contains(msg, "server closed idle connection"):
		return true
	// Treat common HTTP 5xx gateway/availability errors as transient
	case strings.Contains(msg, "status 502"),
		strings.Contains(msg, "502 bad gateway"),
		strings.Contains(msg, "bad gateway"),
		strings.Contains(msg, "status 503"),
		strings.Contains(msg, "service unavailable"),
		strings.Contains(msg, "status 504"),
		strings.Contains(msg, "gateway timeout"):
		return true
	}
	return false
}

// prepareGenerateRequest prepares a GenerateRequest and resolves the model based
// on preferences or defaults. It expands templates, validates input, and clones options.
func (s *Service) prepareGenerateRequest(ctx context.Context, input *GenerateInput) (*llm.GenerateRequest, llm.Model, error) {

	input.MatchModelIfNeeded(s.modelMatcher)
	if input.Binding == nil {
		input.Binding = &prompt.Binding{}
	}
	model, err := s.llmFinder.Find(ctx, input.Model)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find model: %w", err)
	}
	s.updateFlags(input, model)
	if err := input.Init(ctx); err != nil {
		return nil, nil, fmt.Errorf("failed to init generate input: %w", err)
	}
	if err := input.Validate(ctx); err != nil {
		return nil, nil, err
	}

	// Enforce provider capability and per-conversation attachment limits
	if err := s.enforceAttachmentPolicy(ctx, input, model); err != nil {
		return nil, nil, err
	}

	request := &llm.GenerateRequest{
		Messages: input.Message,
		Options:  input.Options,
	}
	if convID := strings.TrimSpace(memory.ConversationIDFromContext(ctx)); convID != "" {
		request.PromptCacheKey = convID
	}
	applyInstructionsDefaults(request, model)

	return request, model, nil
}

func applyInstructionsDefaults(request *llm.GenerateRequest, model llm.Model) {
	if request == nil {
		return
	}
	supportsInstructions := model != nil && model.Implements(base.SupportsInstructions)

	// Derive instructions from the first system message when unset.
	if strings.TrimSpace(request.Instructions) == "" {
		for i, msg := range request.Messages {
			if msg.Role != llm.RoleSystem {
				continue
			}
			text := llm.MessageText(msg)
			if strings.TrimSpace(text) == "" {
				break
			}
			request.Instructions = text
			if supportsInstructions {
				request.Messages = append(request.Messages[:i], request.Messages[i+1:]...)
			}
			break
		}
	}

	// For providers that do not support top-level instructions, ensure the
	// guidance is present as the first system message.
	if !supportsInstructions && strings.TrimSpace(request.Instructions) != "" {
		for _, msg := range request.Messages {
			if msg.Role == llm.RoleSystem {
				return
			}
		}
		request.Messages = append([]llm.Message{llm.NewSystemMessage(request.Instructions)}, request.Messages...)
	}
}

func (s *Service) updateFlags(input *GenerateInput, model llm.Model) {
	input.Binding.Flags.CanUseTool = model.Implements(base.CanUseTools)
	input.Binding.Flags.CanStream = model.Implements(base.CanStream)
	input.Binding.Flags.IsMultimodal = model.Implements(base.IsMultimodal)

	// Gate parallel tool-calls option based on provider/model support.
	// If the agent config requested parallel tool calls but the model
	// doesn’t implement the capability, force-disable it to avoid
	// sending unsupported fields downstream.
	if input.Options != nil && input.Options.ParallelToolCalls {
		if !model.Implements(base.CanExecToolsInParallel) {
			input.Options.ParallelToolCalls = false
		}
	}
}

// tryGenerateContinuationByAnchor performs non-stream continuation calls grouped by
// persisted TraceID (response.id) when enabled. It returns the last response,
// a handled flag, and an error when a subcall fails.
func (s *Service) tryGenerateContinuationByAnchor(ctx context.Context, model llm.Model, request *llm.GenerateRequest) (*llm.GenerateResponse, bool, error) {
	if !IsContextContinuationEnabled(model) {
		return nil, false, nil
	}
	turn, ok := memory.TurnMetaFromContext(ctx)
	if !ok {
		return nil, false, nil
	}
	traces := s.resolveTraces(ctx, turn.ConversationID)
	groups, order, latest := groupMessagesByAnchor(request.Messages, traces)
	if len(groups) == 0 {
		return nil, false, nil
	}
	if latest == "" && len(order) == 1 {
		latest = order[0]
	}
	if latest != "" {
		if msgs, ok := groups[latest]; ok {
			groups = map[string][]llm.Message{latest: msgs}
			order = []string{latest}
		}
	}
	var lastResp *llm.GenerateResponse
	for _, anchor := range order {
		msgs := groups[anchor]
		sub := &llm.GenerateRequest{}
		if request.Options != nil {
			opts := *request.Options
			sub.Options = &opts
		}
		sub.Messages = make([]llm.Message, len(msgs))
		copy(sub.Messages, msgs)
		sub.PreviousResponseID = anchor
		resp, gerr := model.Generate(ctx, sub)
		if gerr != nil {
			if isContextLimitError(gerr) {
				return nil, true, ContinuationContextLimitError{Err: gerr}
			}
			return nil, true, fmt.Errorf("continuation subcall failed: %w", gerr)
		}
		lastResp = resp
	}
	return lastResp, true, nil
}

func groupMessagesByAnchor(messages []llm.Message, traces apiconv.IndexedMessages) (map[string][]llm.Message, []string, string) {
	groups := map[string][]llm.Message{}
	anchorTimes := map[string]time.Time{}
	firstSeen := map[string]int{}
	seenOrder := 0
	var latestAnchor string
	var latestTime time.Time
	getAnchor := func(callID string) string {
		if callID == "" {
			return ""
		}
		if traceMsg, ok := traces[callID]; ok && traceMsg != nil && traceMsg.ToolCall != nil && traceMsg.ToolCall.TraceId != nil {
			return strings.TrimSpace(*traceMsg.ToolCall.TraceId)
		}
		return ""
	}
	appendMsg := func(anchor string, msg llm.Message) {
		if anchor == "" {
			return
		}
		if _, ok := groups[anchor]; !ok {
			firstSeen[anchor] = seenOrder
			seenOrder++
		}
		groups[anchor] = append(groups[anchor], msg)
		if traceMsg, ok := traces[anchor]; ok && traceMsg != nil {
			if traceMsg.ModelCall != nil {
				if latestTime.IsZero() || traceMsg.CreatedAt.After(latestTime) {
					latestTime = traceMsg.CreatedAt
					latestAnchor = anchor
				}
			}
			if traceMsg.CreatedAt.After(anchorTimes[anchor]) || anchorTimes[anchor].IsZero() {
				anchorTimes[anchor] = traceMsg.CreatedAt
			}
		}
	}
	for _, msg := range messages {
		if len(msg.ToolCalls) > 0 {
			byAnchor := map[string][]llm.ToolCall{}
			for _, call := range msg.ToolCalls {
				anchor := getAnchor(strings.TrimSpace(call.ID))
				if anchor == "" {
					continue
				}
				byAnchor[anchor] = append(byAnchor[anchor], call)
			}
			for anchor, calls := range byAnchor {
				copyMsg := msg
				copyMsg.ToolCalls = make([]llm.ToolCall, len(calls))
				copy(copyMsg.ToolCalls, calls)
				appendMsg(anchor, copyMsg)
			}
			continue
		}
		if id := strings.TrimSpace(msg.ToolCallId); id != "" {
			anchor := getAnchor(id)
			if anchor == "" {
				continue
			}
			appendMsg(anchor, msg)
		}
	}
	order := make([]string, 0, len(groups))
	for anchor := range groups {
		order = append(order, anchor)
	}
	sort.Slice(order, func(i, j int) bool {
		iAnchor := order[i]
		jAnchor := order[j]
		ti := anchorTimes[iAnchor]
		tj := anchorTimes[jAnchor]
		switch {
		case !ti.IsZero() && !tj.IsZero():
			if ti.Equal(tj) {
				return firstSeen[iAnchor] < firstSeen[jAnchor]
			}
			return ti.Before(tj)
		case !ti.IsZero():
			return true
		case !tj.IsZero():
			return false
		default:
			return firstSeen[iAnchor] < firstSeen[jAnchor]
		}
	})
	return groups, order, latestAnchor
}

// enforceAttachmentPolicy removes or limits binary content items based on
// model multimodal capability and provider-specific per-conversation caps.
func (s *Service) enforceAttachmentPolicy(ctx context.Context, input *GenerateInput, model llm.Model) error {
	if input == nil || len(input.Message) == 0 {
		return nil
	}
	// 1) Drop all binaries when not multimodal
	isMM := input.Binding != nil && input.Binding.Flags.IsMultimodal
	convID := ""
	if tm, ok := memory.TurnMetaFromContext(ctx); ok {
		convID = tm.ConversationID
	}

	// 2) Provider-specific limit
	// Use provider-reported default if any (currently 0 in core; agent layer enforces caps)
	var limit int64 = s.ProviderAttachmentLimit(model)

	used := int64(0)
	if convID != "" && s.attachUsage != nil {
		used = s.attachUsage[convID]
	}

	var keptBytes int64
	filtered := make([]llm.Message, 0, len(input.Message))
	for _, m := range input.Message {
		if len(m.Items) == 0 {
			filtered = append(filtered, m)
			continue
		}
		newItems := make([]llm.ContentItem, 0, len(m.Items))
		for _, it := range m.Items {
			if it.Type != llm.ContentTypeBinary {
				newItems = append(newItems, it)
				continue
			}
			if !isMM {
				// Skip all binaries when model not multimodal
				continue
			}
			// Estimate raw size for base64
			rawSize := int64(0)
			if it.Source == llm.SourceBase64 && it.Data != "" {
				// base64 decoded length approximation
				if dec, err := base64.StdEncoding.DecodeString(it.Data); err == nil {
					rawSize = int64(len(dec))
				}
			}
			if limit > 0 {
				remain := limit - used - keptBytes
				if remain <= 0 || (rawSize > 0 && rawSize > remain) {
					continue
				}
			}
			newItems = append(newItems, it)
			keptBytes += rawSize
		}
		// Keep message if any item left or it had a text Content
		if len(newItems) > 0 || strings.TrimSpace(m.Content) != "" {
			m.Items = newItems
			filtered = append(filtered, m)
		}
	}
	if convID != "" && s.attachUsage != nil && keptBytes > 0 {
		s.attachUsage[convID] = used + keptBytes
	}
	input.Message = filtered
	// User-facing warnings
	if !isMM {
		fmt.Println("[warning] attachments ignored: selected model is not multimodal")
	} else if limit > 0 && keptBytes < 0 {
		fmt.Println("[warning] attachment limit reached; some files were skipped")
	}
	return nil
}

//
//func attachmentMIME(a *prompt.Attachment) string {
//	if a == nil {
//		return "application/octet-Stream"
//	}
//	if strings.TrimSpace(a.Mime) != "" {
//		return a.Mime
//	}
//	name := strings.TrimSpace(a.Name)
//	if name == "" {
//		return "application/octet-Stream"
//	}
//	ext := strings.ToLower(strings.TrimPrefix(path.Ext(name), "."))
//	switch ext {
//	case "jpg", "jpeg":
//		return "image/jpeg"
//	case "png":
//		return "image/png"
//	case "gif":
//		return "image/gif"
//	case "pdf":
//		return "application/pdf"
//	case "txt":
//		return "text/plain"
//	case "md":
//		return "text/markdown"
//	case "csv":
//		return "text/csv"
//	case "json":
//		return "application/json"
//	case "xml":
//		return "application/xml"
//	case "html":
//		return "text/html"
//	case "yaml", "yml":
//		return "application/x-yaml"
//	case "zip":
//		return "application/zip"
//	case "tar":
//		return "application/x-tar"
//	case "mp3":
//		return "audio/mpeg"
//	case "mp4":
//		return "video/mp4"
//	}
//	return "application/octet-Stream"
//}
