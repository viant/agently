package modelcallctx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/memory"
	agconv "github.com/viant/agently/pkg/agently/conversation"
)

const streamPersistModeEnv = "AGENTLY_STREAM_PERSIST_MODE"

type streamPersistModeOverrideKey struct{}

const (
	streamPersistLegacy = iota
	streamPersistFinal
	streamPersistBuffered
)

const (
	streamPersistBufferedInterval = 300 * time.Millisecond
	streamPersistBufferedMinBytes = 10 * 1024
)

// recorderObserver writes model-call data directly using conversation client.
type recorderObserver struct {
	client          apiconv.Client
	start           Info
	hasBeg          bool
	mu              sync.Mutex
	msgID           string
	ended           bool
	acc             strings.Builder
	streamPayloadID string
	streamLinked    bool
	streamStatusSet bool
	lastFlushAt     time.Time
	lastFlushSize   int
	// Optional: resolve token prices for a model (per 1k tokens).
	priceProvider TokenPriceProvider
}

const finalizePersistTimeout = 15 * time.Second

func (o *recorderObserver) OnCallStart(ctx context.Context, info Info) (context.Context, error) {
	o.start = info
	o.hasBeg = true
	o.acc.Reset()
	o.streamPayloadID = ""
	o.streamLinked = false
	o.streamStatusSet = false
	o.lastFlushAt = time.Time{}
	o.lastFlushSize = 0
	if info.StartedAt.IsZero() {
		o.start.StartedAt = time.Now()
	}
	// Persist a redacted request payload for transcript/logging purposes so large
	// base64 attachments don't overwhelm the conversation payload store.
	if info.LLMRequest != nil {
		if redacted := RedactGenerateRequestForTranscript(info.LLMRequest); len(redacted) > 0 {
			info.Payload = redacted
		}
	}
	// Attach finish barrier so downstream can wait for persistence before emitting final message.
	ctx, _ = WithFinishBarrier(ctx)
	msgID := uuid.NewString()
	ctx = context.WithValue(ctx, memory.ModelMessageIDKey, msgID)
	o.mu.Lock()
	o.msgID = msgID
	o.ended = false
	o.mu.Unlock()
	turn, _ := memory.TurnMetaFromContext(ctx)

	// Create interim assistant message to capture request payload in transcript
	if turn.ConversationID != "" {
		mode := ""
		if info.LLMRequest != nil && info.LLMRequest.Options != nil {
			mode = info.LLMRequest.Options.Mode
		}
		if err := o.patchInterimRequestMessage(ctx, turn, msgID, info.Payload, mode); err != nil {
			return ctx, err
		}
	}
	// Defer assigning stream payload id until first stream chunk,
	// so we can align it with message id to simplify lookups.

	// Start model call and persist request/provider request payloads
	if err := o.beginModelCall(ctx, msgID, turn, info); err != nil {
		return ctx, err
	}
	return ctx, nil
}

func (o *recorderObserver) OnCallEnd(ctx context.Context, info Info) error {
	// Ensure finish barrier is always released to avoid deadlocks.
	defer signalFinish(ctx)

	if !o.hasBeg { // tolerate missing start
		o.start = Info{}
	}
	if info.CompletedAt.IsZero() {
		info.CompletedAt = time.Now()
	}
	// attach to message/turn from context
	msgID := o.resolveMessageID(ctx)
	if msgID == "" {
		return nil
	}
	if o.isEnded(msgID) {
		return nil
	}

	return o.finalizeOpenCall(ctx, msgID, info)
}

// CloseIfOpen force-closes the current model call when it was started but did not
// reach a terminal state. It is used as a fallback from upper layers when providers
// exit early without invoking OnCallEnd.
func (o *recorderObserver) CloseIfOpen(ctx context.Context, info Info) error {
	msgID := o.resolveMessageID(ctx)
	if msgID == "" || o.isEnded(msgID) {
		return nil
	}
	if info.CompletedAt.IsZero() {
		info.CompletedAt = time.Now()
	}
	if strings.TrimSpace(info.Err) == "" {
		if cerr := ctx.Err(); cerr != nil {
			info.Err = cerr.Error()
		} else {
			info.Err = "forced close"
		}
	}
	return o.finalizeOpenCall(ctx, msgID, info)
}

func (o *recorderObserver) resolveMessageID(ctx context.Context) string {
	if msgID := strings.TrimSpace(memory.ModelMessageIDFromContext(ctx)); msgID != "" {
		return msgID
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return strings.TrimSpace(o.msgID)
}

func (o *recorderObserver) isEnded(msgID string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.ended && strings.TrimSpace(o.msgID) == strings.TrimSpace(msgID)
}

func (o *recorderObserver) markEnded(msgID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if strings.TrimSpace(o.msgID) == strings.TrimSpace(msgID) {
		o.ended = true
	}
}

func (o *recorderObserver) finalizeOpenCall(ctx context.Context, msgID string, info Info) error {
	persistCtx, cancelPersist := context.WithTimeout(context.WithoutCancel(ctx), finalizePersistTimeout)
	defer cancelPersist()
	turn, _ := memory.TurnMetaFromContext(persistCtx)

	// Persist assistant content (including tool_calls responses) so the UI can show it.
	// When content exists, clear Interim flag to make it visible in the transcript.
	if info.LLMResponse != nil || len(info.ResponseJSON) > 0 {
		madeVisible, err := o.patchAssistantMessageFromInfo(persistCtx, msgID, info)
		if err != nil {
			warnf("patchAssistantMessageFromInfo failed message=%q err=%v", strings.TrimSpace(msgID), err)
		} else if !madeVisible {
			// Keep interim flag only when there is no user-visible content to render.
			if err := o.patchInterimFlag(persistCtx, msgID); err != nil {
				warnf("patchInterimFlag failed message=%q err=%v", strings.TrimSpace(msgID), err)
			}
		}
	}
	// Prefer provider-supplied stream text; fall back to accumulated chunks
	streamTxt := info.StreamText
	if strings.TrimSpace(streamTxt) == "" {
		streamTxt = o.acc.String()
	}

	// Finish model call with response/providerResponse and stream payload.
	// Conversation terminal status is owned by turn finalization, not per-call
	// model lifecycle events.
	status := "completed"
	// Treat context cancellation and deadlines as terminated.
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		status = "canceled"
	} else if strings.TrimSpace(info.Err) != "" {
		lowErr := strings.ToLower(strings.TrimSpace(info.Err))
		if strings.Contains(lowErr, "context canceled") || strings.Contains(lowErr, "context deadline exceeded") {
			status = "canceled"
		} else {
			status = "failed"
		}
	}

	errs := make([]error, 0, 1)
	if err := o.finishModelCall(persistCtx, msgID, status, info, streamTxt); err != nil {
		errs = append(errs, fmt.Errorf("finish model call: %w", err))
	}
	if err := o.persistOpenAIGeneratedFiles(persistCtx, msgID, turn, info); err != nil {
		warnf("persistOpenAIGeneratedFiles failed message=%q err=%v", strings.TrimSpace(msgID), err)
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	o.markEnded(msgID)
	return nil
}

func (o *recorderObserver) patchAssistantMessageFromInfo(ctx context.Context, msgID string, info Info) (bool, error) {
	if strings.TrimSpace(msgID) == "" {
		return false, nil
	}
	resp := info.LLMResponse
	if resp == nil && len(info.ResponseJSON) > 0 {
		// Best-effort decode of response JSON (some providers omit LLMResponse but do provide a JSON snapshot).
		var decoded llm.GenerateResponse
		if err := json.Unmarshal(info.ResponseJSON, &decoded); err == nil {
			resp = &decoded
		}
	}
	content, hasToolCalls := assistantContentFromGenerateResponse(resp)
	content = strings.TrimSpace(content)
	if content == "" {
		return false, nil
	}
	msg := apiconv.NewMessage()
	msg.SetId(msgID)
	if turn, ok := memory.TurnMetaFromContext(ctx); ok && strings.TrimSpace(turn.ConversationID) != "" {
		msg.SetConversationID(turn.ConversationID)
		if strings.TrimSpace(turn.TurnID) != "" {
			msg.SetTurnID(turn.TurnID)
		}
	}
	// Store content always. Store raw_content only for tool-call responses so
	// transcripts can distinguish tool-driven interim content from normal replies.
	msg.SetContent(content)
	if hasToolCalls {
		msg.SetRawContent(content)
		msg.SetInterim(1)
	} else {
		msg.SetInterim(0)
	}
	if err := o.client.PatchMessage(ctx, msg); err != nil {
		return false, err
	}
	return true, nil
}

func assistantContentFromGenerateResponse(resp *llm.GenerateResponse) (string, bool) {
	if resp == nil || len(resp.Choices) == 0 {
		return "", false
	}
	parts := make([]string, 0, len(resp.Choices))
	hasToolCalls := false
	// "finish_reason": "tool_calls"

	for _, c := range resp.Choices {
		if len(c.Message.ToolCalls) > 0 || c.Message.FunctionCall != nil {
			hasToolCalls = true
		}
		if strings.Contains(strings.ToLower(c.FinishReason), "tool") {
			hasToolCalls = true
		}
		s := strings.TrimSpace(messageText(c.Message))
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n")), hasToolCalls
}

func messageText(msg llm.Message) string {
	if s := strings.TrimSpace(msg.Content); s != "" {
		return s
	}
	// Prefer Items; fall back to legacy ContentItems.
	items := msg.Items
	if len(items) == 0 {
		items = msg.ContentItems
	}
	var parts []string
	for _, it := range items {
		if strings.TrimSpace(string(it.Type)) != "" && it.Type != llm.ContentTypeText {
			continue
		}
		if s := strings.TrimSpace(it.Text); s != "" {
			parts = append(parts, s)
			continue
		}
		// Some adapters put text into Data for raw source.
		if s := strings.TrimSpace(it.Data); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

// OnStreamDelta aggregates streamed chunks. Persistence strategy is controlled
// by a context override when present, otherwise AGENTLY_STREAM_PERSIST_MODE:
//   - legacy (default): upsert full accumulated stream on every delta
//   - buffered: periodic full flush from in-memory buffer
//   - final: persist only once on FinishModelCall
func (o *recorderObserver) OnStreamDelta(ctx context.Context, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	o.publishStreamDelta(ctx, data)
	o.acc.Write(data)
	msgID := memory.ModelMessageIDFromContext(ctx)
	mode := streamPersistMode(ctx)

	// Attempt to detect provider response id early from OpenAI Responses events.
	// We look for {"type":"response.created|response.completed","response":{"id":"..."}}.
	var probe struct {
		Type     string `json:"type"`
		Response struct {
			ID string `json:"id"`
		} `json:"response"`
	}
	// Fast-path check to avoid expensive JSON unmarshal on tiny chunks.
	// The minimal JSON that would satisfy the probe is:
	// {"type":"response.created","response":{"id":"x"}} 49 bytes or 48 when id is empty
	if len(data) >= 48 {
		if err := json.Unmarshal(data, &probe); err == nil {
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(probe.Type)), "response.") && strings.TrimSpace(probe.Response.ID) != "" {
				// Always cache in-memory per-turn for quick reuse.
				if turn, ok := memory.TurnMetaFromContext(ctx); ok {
					memory.SetTurnTrace(turn.TurnID, strings.TrimSpace(probe.Response.ID))
				}
				// In non-final modes, also persist trace id early (one DB write).
				if mode != streamPersistFinal && strings.TrimSpace(msgID) != "" {
					upd := apiconv.NewModelCall()
					upd.SetMessageID(msgID)
					upd.SetTraceID(strings.TrimSpace(probe.Response.ID))
					_ = o.client.PatchModelCall(ctx, upd)
				}
			}
		}
	}

	// (1) Per-delta persistence is best-effort. If the turn context is already
	// canceled, keep accumulating in-memory only. Finalization uses a detached
	// context and will persist the full partial stream from o.acc.
	if ctx.Err() != nil {
		return nil
	}

	// (2) Mark model call as streaming once for status visibility. This remains
	// best-effort because a status write failure should not abort the stream.
	if !o.streamStatusSet {
		o.streamStatusSet = true
		if strings.TrimSpace(msgID) != "" {
			upd := apiconv.NewModelCall()
			upd.SetMessageID(msgID)
			upd.SetStatus("streaming")
			if err := o.client.PatchModelCall(ctx, upd); err != nil {
				warnf("patchModelCall streaming status failed message=%q err=%v", strings.TrimSpace(msgID), err)
			}
		}
	}

	switch mode {
	case streamPersistFinal:
		return nil
	case streamPersistBuffered:
		return o.handleStreamDeltaBuffered(ctx, msgID)
	default:
		// Legacy mode below.
	}

	// Legacy mode: per-delta persistence still happens, but we now rebuild from
	// the in-memory accumulator instead of read-append-rewrite via GetPayload.
	// This avoids extra DB reads and keeps the full partial stream available
	// even when an intermediate write is skipped or fails.

	// (3) Resolve stream payload id (message id or new UUID).
	id := strings.TrimSpace(o.streamPayloadID)
	if id == "" {
		if msgID := strings.TrimSpace(memory.ModelMessageIDFromContext(ctx)); msgID != "" {
			id = msgID
		} else {
			id = uuid.New().String()
		}
		o.streamPayloadID = id
	}

	// (4) Upsert the full accumulated stream body. Failures are logged and
	// ignored so persistence issues do not abort the provider stream.
	if _, err := o.upsertInlinePayload(ctx, id, "model_stream", "text/plain", []byte(o.acc.String())); err != nil {
		warnf("stream delta payload update failed message=%q err=%v", strings.TrimSpace(msgID), err)
		return nil
	}

	// (5) Link stream payload to model call once. This is also best-effort.
	if !o.streamLinked {
		if strings.TrimSpace(msgID) != "" {
			upd := apiconv.NewModelCall()
			upd.SetMessageID(msgID)
			upd.SetStreamPayloadID(id)
			if err := o.client.PatchModelCall(ctx, upd); err != nil {
				warnf("stream payload link failed message=%q payload=%q err=%v", strings.TrimSpace(msgID), strings.TrimSpace(id), err)
				return nil
			}
			o.streamLinked = true
		}
	}
	return nil
}

func (o *recorderObserver) handleStreamDeltaBuffered(ctx context.Context, msgID string) error {
	// Buffered mode: persist periodically from the in-memory accumulator instead
	// of on every delta. Finalization still writes the full stream at the end.

	// (3) Resolve stream payload id (message id or new UUID).
	id := strings.TrimSpace(o.streamPayloadID)
	if id == "" {
		if strings.TrimSpace(msgID) != "" {
			id = strings.TrimSpace(msgID)
		} else {
			id = uuid.New().String()
		}
		o.streamPayloadID = id
	}

	now := time.Now()
	accSize := o.acc.Len()

	// (4) First delta starts the buffering window; defer the initial write until
	// either enough time passes or enough new bytes accumulate.
	if o.lastFlushAt.IsZero() {
		o.lastFlushAt = now
		o.lastFlushSize = accSize
		return nil
	}

	// (5) Flush only when the buffer meaningfully advanced since the last write.
	if now.Sub(o.lastFlushAt) < streamPersistBufferedInterval && accSize-o.lastFlushSize < streamPersistBufferedMinBytes {
		return nil
	}

	// (6) Upsert the full accumulated stream body. Failures are logged and
	// ignored so persistence issues do not abort the provider stream.
	if _, err := o.upsertInlinePayload(ctx, id, "model_stream", "text/plain", []byte(o.acc.String())); err != nil {
		warnf("buffered stream payload update failed message=%q err=%v", strings.TrimSpace(msgID), err)
		return nil
	}
	o.lastFlushAt = now
	o.lastFlushSize = accSize

	// (7) Link stream payload to model call once. This is also best-effort.
	if !o.streamLinked && strings.TrimSpace(msgID) != "" {
		upd := apiconv.NewModelCall()
		upd.SetMessageID(msgID)
		upd.SetStreamPayloadID(id)
		if err := o.client.PatchModelCall(ctx, upd); err != nil {
			warnf("buffered stream payload link failed message=%q payload=%q err=%v", strings.TrimSpace(msgID), strings.TrimSpace(id), err)
			return nil
		}
		o.streamLinked = true
	}
	return nil
}

func (o *recorderObserver) publishStreamDelta(ctx context.Context, data []byte) {
	pub, ok := StreamPublisherFromContext(ctx)
	if !ok {
		return
	}
	if looksLikeElicitationDelta(data) {
		return
	}
	convID := strings.TrimSpace(memory.ConversationIDFromContext(ctx))
	if convID == "" {
		return
	}
	msgID := strings.TrimSpace(memory.ModelMessageIDFromContext(ctx))
	if msgID == "" {
		return
	}
	msg := &agconv.MessageView{
		Id:             msgID,
		ConversationId: convID,
		Role:           "assistant",
		Type:           "text",
	}
	if turn, ok := memory.TurnMetaFromContext(ctx); ok {
		if strings.TrimSpace(turn.TurnID) != "" {
			msg.TurnId = &turn.TurnID
		}
		if strings.TrimSpace(turn.ParentMessageID) != "" {
			msg.ParentMessageId = &turn.ParentMessageID
		}
	}
	interim := 1
	msg.Interim = interim
	content := map[string]interface{}{
		"delta": string(data),
		"meta": map[string]interface{}{
			"kind": "stream_delta",
		},
	}
	_ = pub.Publish(ctx, &StreamEvent{
		ConversationID: convID,
		Message:        msg,
		ContentType:    "application/json",
		Content:        content,
	})
}

func looksLikeElicitationDelta(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	raw := strings.ToLower(strings.TrimSpace(string(data)))
	if raw == "" {
		return false
	}
	if strings.HasPrefix(raw, "```json") {
		return true
	}
	if strings.Contains(raw, "\"requestedSchema\"") && strings.Contains(raw, "\"type\"") && strings.Contains(raw, "elicitation") {
		return true
	}
	if strings.Contains(raw, "\"type\"") && strings.Contains(raw, "elicitation") && strings.HasPrefix(raw, "{") {
		return true
	}
	return false
}

// WithRecorderObserver injects a recorder-backed Observer into context.
func WithRecorderObserver(ctx context.Context, client apiconv.Client) context.Context {
	_, ok := memory.TurnMetaFromContext(ctx) //ensure turn is in context
	if !ok {
		ctx = memory.WithTurnMeta(ctx, memory.TurnMeta{
			TurnID:          uuid.New().String(),
			ConversationID:  memory.ConversationIDFromContext(ctx),
			ParentMessageID: memory.ModelMessageIDFromContext(ctx),
		})
	}
	return WithObserver(ctx, &recorderObserver{client: client})
}

// WithRecorderObserverWithPrice injects a recorder-backed Observer with an optional
// price resolver used to compute per-call cost from token usage.
// TokenPriceProvider exposes per-1k token pricing for a model id/name.
type TokenPriceProvider interface {
	TokenPrices(model string) (in float64, out float64, cached float64, ok bool)
}

func WithRecorderObserverWithPrice(ctx context.Context, client apiconv.Client, provider TokenPriceProvider) context.Context {
	_, ok := memory.TurnMetaFromContext(ctx)
	if !ok {
		ctx = memory.WithTurnMeta(ctx, memory.TurnMeta{
			TurnID:          uuid.New().String(),
			ConversationID:  memory.ConversationIDFromContext(ctx),
			ParentMessageID: memory.ModelMessageIDFromContext(ctx),
		})
	}
	return WithObserver(ctx, &recorderObserver{client: client, priceProvider: provider})
}

// patchInterimRequestMessage creates an interim assistant message capturing the request payload.
func (o *recorderObserver) patchInterimRequestMessage(ctx context.Context, turn memory.TurnMeta, msgID string, payload []byte, mode string) error {
	debugf("patchInterimRequestMessage start convo=%q turn=%q msg=%q mode=%q payload_bytes=%d", strings.TrimSpace(turn.ConversationID), strings.TrimSpace(turn.TurnID), strings.TrimSpace(msgID), strings.TrimSpace(mode), len(payload))
	_, err := apiconv.AddMessage(ctx, o.client, &turn,
		apiconv.WithId(msgID),
		apiconv.WithMode(mode),
		apiconv.WithRole("assistant"),
		apiconv.WithType("text"),
		apiconv.WithCreatedByUserID(turn.Assistant),
		apiconv.WithInterim(1),
	)
	if err != nil {
		errorf("patchInterimRequestMessage error convo=%q turn=%q msg=%q err=%v", strings.TrimSpace(turn.ConversationID), strings.TrimSpace(turn.TurnID), strings.TrimSpace(msgID), err)
	} else {
		debugf("patchInterimRequestMessage ok convo=%q turn=%q msg=%q", strings.TrimSpace(turn.ConversationID), strings.TrimSpace(turn.TurnID), strings.TrimSpace(msgID))
	}
	return err
}

// patchInterimFlag marks an existing message as interim.
func (o *recorderObserver) patchInterimFlag(ctx context.Context, msgID string) error {
	interim := 1
	msg := apiconv.NewMessage()
	msg.SetId(msgID)
	// Ensure conversation id present for patching
	if turn, ok := memory.TurnMetaFromContext(ctx); ok && strings.TrimSpace(turn.ConversationID) != "" {
		msg.SetConversationID(turn.ConversationID)
	}
	msg.SetInterim(interim)
	err := o.client.PatchMessage(ctx, msg)
	if err != nil {
		errorf("patchInterimFlag error msg=%q err=%v", strings.TrimSpace(msgID), err)
	} else {
		debugf("patchInterimFlag ok msg=%q", strings.TrimSpace(msgID))
	}
	return err
}

//298c12dc-d9d9-45d1-b340-09611803c940

// beginModelCall persists the initial model call and associated request payloads.
func (o *recorderObserver) beginModelCall(ctx context.Context, msgID string, turn memory.TurnMeta, info Info) error {
	debugf("beginModelCall start convo=%q turn=%q msg=%q provider=%q model=%q kind=%q req_bytes=%d provider_req_bytes=%d", strings.TrimSpace(turn.ConversationID), strings.TrimSpace(turn.TurnID), strings.TrimSpace(msgID), strings.TrimSpace(info.Provider), strings.TrimSpace(info.Model), strings.TrimSpace(info.ModelKind), len(info.Payload), len(info.RequestJSON))
	mc := apiconv.NewModelCall()
	mc.SetMessageID(msgID)
	if turn.TurnID != "" {
		mc.SetTurnID(turn.TurnID)
	}
	mc.SetProvider(info.Provider)
	mc.SetModel(info.Model)
	if strings.TrimSpace(info.ModelKind) != "" {
		mc.SetModelKind(info.ModelKind)
	}
	mc.SetStatus("thinking")
	t := o.start.StartedAt
	mc.SetStartedAt(t)

	// request payload
	if len(info.Payload) > 0 {
		reqID, err := o.upsertInlinePayload(ctx, "", "model_request", "application/json", info.Payload)
		if err != nil {
			errorf("beginModelCall request payload error msg=%q err=%v", strings.TrimSpace(msgID), err)
			return err
		}
		mc.SetRequestPayloadID(reqID)
	}
	// provider request snapshot
	if len(info.RequestJSON) > 0 {
		prID, err := o.upsertInlinePayload(ctx, "", "provider_request", "application/json", info.RequestJSON)
		if err != nil {
			errorf("beginModelCall provider payload error msg=%q err=%v", strings.TrimSpace(msgID), err)
			return err
		}
		mc.SetProviderRequestPayloadID(prID)
	}
	// Do not link stream payload at start to avoid FK violation.
	// Stream payload link will be set after the payload is created (OnStreamDelta/OnCallEnd).
	if err := o.client.PatchModelCall(ctx, mc); err != nil {
		errorf("beginModelCall patch model call error msg=%q err=%v", strings.TrimSpace(msgID), err)
		return err
	}
	debugf("beginModelCall ok convo=%q turn=%q msg=%q", strings.TrimSpace(turn.ConversationID), strings.TrimSpace(turn.TurnID), strings.TrimSpace(msgID))
	return nil
}

// finishModelCall persists final model call updates, including response payloads and usage.
func (o *recorderObserver) finishModelCall(ctx context.Context, msgID, status string, info Info, streamTxt string) error {
	hasResp := info.LLMResponse != nil
	debugf("finishModelCall start msg=%q status=%q has_llm_response=%v provider_resp_bytes=%d stream_bytes=%d", strings.TrimSpace(msgID), strings.TrimSpace(status), hasResp, len(info.ResponseJSON), len(streamTxt))
	upd := apiconv.NewModelCall()
	upd.SetMessageID(msgID)
	upd.SetStatus(status)
	if strings.TrimSpace(info.Err) != "" {
		upd.SetErrorMessage(info.Err)
	}
	if strings.TrimSpace(info.ErrorCode) != "" {
		upd.SetErrorCode(info.ErrorCode)
	}
	if !info.CompletedAt.IsZero() {
		upd.SetCompletedAt(info.CompletedAt)
	}

	// persist response payload snapshot
	if info.LLMResponse != nil {
		if rb, mErr := json.Marshal(info.LLMResponse); mErr == nil {
			respID, err := o.upsertInlinePayload(ctx, "", "model_response", "application/json", rb)
			if err != nil {
				errorf("finishModelCall response payload error msg=%q err=%v", strings.TrimSpace(msgID), err)
				return err
			}
			upd.SetResponsePayloadID(respID)
		}
		// Set trace (provider response id) for continuation
		if strings.TrimSpace(info.LLMResponse.ResponseID) != "" {
			upd.SetTraceID(strings.TrimSpace(info.LLMResponse.ResponseID))
			if turn, ok := memory.TurnMetaFromContext(ctx); ok {
				memory.SetTurnTrace(turn.TurnID, strings.TrimSpace(info.LLMResponse.ResponseID))
			}
		}
	}
	if len(info.ResponseJSON) > 0 {
		provID, err := o.upsertInlinePayload(ctx, "", "provider_response", "application/json", []byte(info.ResponseJSON))
		if err != nil {
			errorf("finishModelCall provider response payload error msg=%q err=%v", strings.TrimSpace(msgID), err)
			return err
		}
		upd.SetProviderResponsePayloadID(provID)
	}
	if strings.TrimSpace(streamTxt) != "" {
		sid := strings.TrimSpace(o.streamPayloadID)
		if sid == "" {
			sid = uuid.New().String()
		}
		if _, err := o.upsertInlinePayload(ctx, sid, "model_stream", "text/plain", []byte(streamTxt)); err != nil {
			errorf("finishModelCall stream payload error msg=%q err=%v", strings.TrimSpace(msgID), err)
			return err
		}
		upd.SetStreamPayloadID(sid)
	}

	// usage mapping
	if info.Usage != nil {
		u := info.Usage
		if u.PromptTokens > 0 {
			upd.SetPromptTokens(u.PromptTokens)
		}
		if u.CompletionTokens > 0 {
			upd.SetCompletionTokens(u.CompletionTokens)
		}
		if u.TotalTokens > 0 {
			upd.SetTotalTokens(u.TotalTokens)
		}
		if u.CachedTokens > 0 {
			upd.SetPromptCachedTokens(u.CachedTokens)
		}
		// Compute call cost when a price resolver is available and prices are defined
		if o.priceProvider != nil {
			inP, outP, cachedP, ok := o.priceProvider.TokenPrices(strings.TrimSpace(info.Model))
			if !ok {
				debugPricingf("no prices found for model=%s", strings.TrimSpace(info.Model))
			}
			if ok {
				// Prefer provider-supplied cached tokens; tolerate zero
				cached := u.CachedTokens
				if cached == 0 && u.PromptCachedTokens > 0 {
					cached = u.PromptCachedTokens
				}
				cost := (float64(u.PromptTokens)*inP + float64(u.CompletionTokens)*outP + float64(cached)*cachedP) / 1000.0
				if cost > 0 {
					upd.SetCost(cost)
					debugPricingf("computed cost model=%s in=%.6f out=%.6f cached=%.6f -> cost=%.6f", strings.TrimSpace(info.Model), inP, outP, cachedP, cost)
				} else {
					debugPricingf("computed zero/negative cost model=%s in=%.6f out=%.6f cached=%.6f", strings.TrimSpace(info.Model), inP, outP, cachedP)
				}
			}
		} else {
			debugPricingf("price provider not set; skipping cost for model=%s", strings.TrimSpace(info.Model))
		}
	}
	if err := o.client.PatchModelCall(ctx, upd); err != nil {
		errorf("finishModelCall patch model call error msg=%q err=%v", strings.TrimSpace(msgID), err)
		return err
	}
	debugf("finishModelCall ok msg=%q status=%q", strings.TrimSpace(msgID), strings.TrimSpace(status))
	return nil
}

// upsertInlinePayload creates or updates an inline payload and returns its id.
// If id is empty, a new id is generated.
func (o *recorderObserver) upsertInlinePayload(ctx context.Context, id, kind, mime string, body []byte) (string, error) {
	if strings.TrimSpace(id) == "" {
		id = uuid.New().String()
	}
	debugf("upsertInlinePayload start id=%q kind=%q mime=%q size_bytes=%d", strings.TrimSpace(id), strings.TrimSpace(kind), strings.TrimSpace(mime), len(body))
	pw := apiconv.NewPayload()
	pw.SetId(id)
	pw.SetKind(kind)
	pw.SetMimeType(mime)
	pw.SetSizeBytes(len(body))
	pw.SetStorage("inline")
	pw.SetInlineBody(body)
	if err := o.client.PatchPayload(ctx, pw); err != nil {
		errorf("upsertInlinePayload error id=%q err=%v", strings.TrimSpace(id), err)
		return "", err
	}
	debugf("upsertInlinePayload ok id=%q", strings.TrimSpace(id))
	return id, nil
}

// --- transient debug helpers (enabled with AGENTLY_DEBUG_PRICING=1) ---
func debugPricingEnabled() bool { return os.Getenv("AGENTLY_DEBUG_PRICING") == "1" }
func debugPricingf(format string, args ...interface{}) {
	if !debugPricingEnabled() {
		return
	}
	fmt.Printf("[pricing] "+format+"\n", args...)
}

// WithStreamPersistMode overrides stream persistence mode for the current call
// context. Invalid values are ignored so callers can safely pass best-effort
// scheduler hints.
func WithStreamPersistMode(ctx context.Context, mode string) context.Context {
	parsed, ok := parseStreamPersistMode(mode)
	if !ok {
		return ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, streamPersistModeOverrideKey{}, parsed)
}

// streamPersistMode returns the selected persistence mode. Context override
// wins over AGENTLY_STREAM_PERSIST_MODE.
func streamPersistMode(ctx context.Context) int {
	if ctx != nil {
		if mode, ok := ctx.Value(streamPersistModeOverrideKey{}).(int); ok {
			return mode
		}
	}
	if mode, ok := parseStreamPersistMode(os.Getenv(streamPersistModeEnv)); ok {
		return mode
	}
	return streamPersistLegacy
}

func parseStreamPersistMode(v string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "legacy":
		return streamPersistLegacy, true
	case "final", "finish", "onfinish":
		return streamPersistFinal, true
	case "buffered":
		return streamPersistBuffered, true
	default:
		return 0, false
	}
}
