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

	if i.Binding != nil && len(i.Binding.History.Messages) > 0 {
		messages := i.Binding.History.Messages
		for k := 0; k < len(messages); k++ {
			m := messages[k]
			if len(m.Attachment) > 0 {
				sortAttachments(m.Attachment)

				var attachItems []*llm.AttachmentItem
				for _, a := range m.Attachment {
					item := &llm.AttachmentItem{Name: a.Name, Data: a.Data, Content: a.Content, MimeType: a.MIMEType()}
					attachItems = append(attachItems, item)
				}

				bMsg := llm.NewMessageWithBinaries(llm.MessageRole(m.Role), attachItems, m.Content)
				i.Message = append(i.Message, bMsg)
			} else {
				i.Message = append(i.Message, llm.NewTextMessage(llm.MessageRole(m.Role), m.Content))
			}
		}
	}

	if tools := i.Binding.Tools; len(tools.Signatures) > 0 {
		for _, tool := range tools.Signatures {
			i.Options.Tools = append(i.Options.Tools, llm.Tool{Type: "function", Ref: "", Definition: *tool})
		}
		for _, call := range tools.Executions {
			msg := llm.NewAssistantMessageWithToolCalls(*call)
			if strings.TrimSpace(i.AgentID) != "" {
				msg.Name = i.AgentID
			}
			i.Message = append(i.Message, msg)
			i.Message = append(i.Message, llm.NewToolResultMessage(*call))
		}
	}

	// Append current user prompt with attributed name when available

	var userMsg llm.Message

	// Include task-scoped attachments for this turn (if any) before the user prompt
	if i.Binding != nil && len(i.Binding.Task.Attachments) > 0 {
		sortAttachments(i.Binding.Task.Attachments)

		var attachItems []*llm.AttachmentItem
		for _, a := range i.Binding.Task.Attachments {
			item := &llm.AttachmentItem{Name: a.Name, Data: a.Data, Content: a.Content, MimeType: a.MIMEType()}
			attachItems = append(attachItems, item)
		}

		userMsg = llm.NewMessageWithBinaries(llm.RoleUser, attachItems, currentPrompt)

	} else {
		userMsg = llm.NewUserMessage(currentPrompt)
	}

	if strings.TrimSpace(i.UserID) != "" {
		userMsg.Name = i.UserID
	}

	i.Message = append(i.Message, userMsg)

	// replacing trace for original user query (prompt) with expanded prompt
	// history doesn't have user query (first request in current turn), we use task.Prompt as source to avoid duplicate messages and attachments
	if i.Binding.History.Traces != nil {
		ckeyOriginal := prompt.KindContent.Key(i.Binding.Task.Prompt)
		origTrace, ok := i.Binding.History.Traces[ckeyOriginal]

		if ok {
			ckey := prompt.KindContent.Key(currentPrompt)
			i.Binding.History.Traces[ckey] = &prompt.Trace{ID: ckey, Kind: prompt.KindContent, At: origTrace.At}
			delete(i.Binding.History.Traces, ckeyOriginal)
		}
	}

	if len(i.Binding.History.UserElicitation) > 0 {
		for _, elicitationMsg := range i.Binding.History.UserElicitation {
			i.Message = append(i.Message, llm.NewTextMessage(llm.MessageRole(elicitationMsg.Role), elicitationMsg.Content))
			// Debug: keys or a short sample
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

	// If provider supports continuation by response id, only set it when the
	// request carries tool results and all of them share a single anchor.
	// This avoids mixing outputs from different response anchors and prevents
	// "No tool output found for function call" errors.
	if IsContextContinuationEnabled(model) {
		if turn, ok := memory.TurnMetaFromContext(ctx); ok {
			// Collect anchors for all tool-result messages in this request
			anchors := map[string]struct{}{}
			totalTools := 0
			missing := 0
			// Resolve persisted traces for opIDs
			traces := s.resolveTraces(ctx, turn.ConversationID)
			for _, m := range request.Messages {
				if strings.TrimSpace(m.ToolCallId) == "" && !strings.EqualFold(string(m.Role), "tool") {
					continue
				}
				totalTools++
				callID := strings.TrimSpace(m.ToolCallId)
				var anchor string
				if msg, ok := traces[callID]; ok && msg != nil && msg.ToolCall != nil && msg.ToolCall.TraceId != nil {
					anchor = strings.TrimSpace(*msg.ToolCall.TraceId)
				}
				if anchor == "" {
					missing++
					continue
				}
				anchors[anchor] = struct{}{}
			}

			if totalTools > 0 && missing == 0 && len(anchors) == 1 {
				// Safe: all tool results point to the same previous response
				for a := range anchors {
					request.PreviousResponseID = a
				}
			}
		}
	}
	return request, model, nil
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
	groups := map[string][]llm.Message{}
	toolFound := false
	var totalTools, missing int
	var selectedPrev string
	var turn memory.TurnMeta
	var ok bool
	if turn, ok = memory.TurnMetaFromContext(ctx); ok {
		traces := s.resolveTraces(ctx, turn.ConversationID)
		for _, m := range request.Messages {
			if strings.TrimSpace(m.ToolCallId) == "" {
				continue
			}
			totalTools++
			callID := strings.TrimSpace(m.ToolCallId)
			var anchor string
			if msg, ok := traces[callID]; ok && msg != nil && msg.ToolCall != nil && msg.ToolCall.TraceId != nil {
				anchor = strings.TrimSpace(*msg.ToolCall.TraceId)
			}
			if anchor == "" {
				missing++
				continue
			}
			toolFound = true
			groups[anchor] = append(groups[anchor], m)
		}
		// Prefer the latest assistant response (model call) present in groups by persisted resp.id timing.
		var latestTime time.Time
		for respID := range groups {
			if msg, ok := traces[respID]; ok && msg != nil && msg.ModelCall != nil {
				if latestTime.IsZero() || msg.CreatedAt.After(latestTime) {
					latestTime = msg.CreatedAt
					selectedPrev = respID
				}
			}
		}
	}
	if !toolFound || len(groups) == 0 {
		return nil, false, nil
	}
	// Fallback: if there is exactly one group, pick it.
	if selectedPrev == "" && len(groups) == 1 {
		for k := range groups {
			selectedPrev = k
		}
	}
	// Narrow groups to the selected anchor if determined.
	if selectedPrev != "" {
		if msgs, ok := groups[selectedPrev]; ok {
			groups = map[string][]llm.Message{selectedPrev: msgs}
		}
	}
	order := make([]string, 0, len(groups))
	for k := range groups {
		order = append(order, k)
	}
	sort.Strings(order)
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
			// Fallback: on token limit, retry without tool calls and without previous_response_id
			if isContextLimitError(gerr) {
				fb := &llm.GenerateRequest{}
				if request.Options != nil {
					opts := *request.Options
					fb.Options = &opts
				}
				// Filter out tool messages (role tool or with ToolCallId)
				for _, m := range request.Messages {
					if strings.EqualFold(string(m.Role), "tool") {
						continue
					}
					if strings.TrimSpace(m.ToolCallId) != "" {
						continue
					}
					fb.Messages = append(fb.Messages, m)
				}
				// Ensure no previous_response_id on fallback
				fb.PreviousResponseID = ""
				fresp, ferr := model.Generate(ctx, fb)
				if ferr == nil {
					return fresp, true, nil
				}
			}
			return nil, true, fmt.Errorf("continuation subcall failed: %w", gerr)
		}
		lastResp = resp
	}
	return lastResp, true, nil
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
