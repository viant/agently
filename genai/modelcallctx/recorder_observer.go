package modelcallctx

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/memory"
	agconv "github.com/viant/agently/pkg/agently/conversation"
	convw "github.com/viant/agently/pkg/agently/conversation/write"
)

// recorderObserver writes model-call data directly using conversation client.
type recorderObserver struct {
	client          apiconv.Client
	start           Info
	hasBeg          bool
	acc             strings.Builder
	streamPayloadID string
	streamLinked    bool
	streamStatusSet bool
	// Optional: resolve token prices for a model (per 1k tokens).
	priceProvider TokenPriceProvider
}

func (o *recorderObserver) OnCallStart(ctx context.Context, info Info) (context.Context, error) {
	o.start = info
	o.hasBeg = true
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
	turn, _ := memory.TurnMetaFromContext(ctx)
	// attach to message/turn from context
	msgID := memory.ModelMessageIDFromContext(ctx)
	if msgID == "" {
		return nil
	}

	// Persist assistant content (including tool_calls responses) so the UI can show it.
	// When content exists, clear Interim flag to make it visible in the transcript.
	if info.LLMResponse != nil || len(info.ResponseJSON) > 0 {
		madeVisible, err := o.patchAssistantMessageFromInfo(ctx, msgID, info)
		if err != nil {
			return err
		}
		// Keep interim flag only when there is no user-visible content to render.
		if !madeVisible {
			if err := o.patchInterimFlag(ctx, msgID); err != nil {
				return err
			}
		}
	}
	// Prefer provider-supplied stream text; fall back to accumulated chunks
	streamTxt := info.StreamText
	if strings.TrimSpace(streamTxt) == "" {
		streamTxt = o.acc.String()
	}

	// Finish model call with response/providerResponse and stream payload
	status := "completed"
	// Treat context cancellation as terminated
	if ctx.Err() == context.Canceled {
		status = "canceled"
	} else if strings.TrimSpace(info.Err) != "" {
		status = "failed"
	}

	// Use background context for persistence when terminated to avoid cancellation issues
	finCtx := ctx
	if status == "canceled" {
		finCtx = context.Background()
	}
	if err := o.finishModelCall(finCtx, msgID, status, info, streamTxt); err != nil {
		return err
	}
	if err := o.client.PatchConversations(ctx, convw.NewConversationStatus(turn.ConversationID, status)); err != nil {
		return fmt.Errorf("failed to update conversation: %w", err)
	}
	if err := o.persistOpenAIGeneratedFiles(ctx, msgID, turn, info); err != nil {
		warnf("persistOpenAIGeneratedFiles failed message=%q err=%v", strings.TrimSpace(msgID), err)
	}
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
// by AGENTLY_STREAM_PERSIST_MODE:
//   - legacy (default): append+upsert on every delta
//   - final: persist only once on FinishModelCall
func (o *recorderObserver) OnStreamDelta(ctx context.Context, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	o.publishStreamDelta(ctx, data)
	o.acc.Write(data)
	msgID := memory.ModelMessageIDFromContext(ctx)

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
				// In legacy mode, also persist trace id early (one DB write).
				if !streamPersistFinalOnly() && strings.TrimSpace(msgID) != "" {
					upd := apiconv.NewModelCall()
					upd.SetMessageID(msgID)
					upd.SetTraceID(strings.TrimSpace(probe.Response.ID))
					_ = o.client.PatchModelCall(ctx, upd)
				}
			}
		}
	}
	if streamPersistFinalOnly() {
		// (1) Final-only mode: skip per-delta persistence.
		// (2) Best-effort: mark model call as streaming once for status visibility.
		if !o.streamStatusSet {
			if strings.TrimSpace(msgID) != "" {
				upd := apiconv.NewModelCall()
				upd.SetMessageID(msgID)
				upd.SetStatus("streaming")
				_ = o.client.PatchModelCall(ctx, upd)
			}
			o.streamStatusSet = true
		}
		return nil
	}
	// Legacy mode: per-delta persistence (read + append + full rewrite).
	// (1) Resolve stream payload id (message id or new UUID).
	id := strings.TrimSpace(o.streamPayloadID)
	if id == "" {
		// Prefer using message id as stream payload id on first chunk
		if msgID := strings.TrimSpace(memory.ModelMessageIDFromContext(ctx)); msgID != "" {
			id = msgID
		} else {
			id = uuid.New().String()
		}
		o.streamPayloadID = id

	}

	// (2) Load current payload body (GetPayload).
	var cur []byte
	pv, err := o.client.GetPayload(ctx, id)
	if err == nil && pv != nil && pv.InlineBody != nil {
		cur = *pv.InlineBody
	}
	if pv == nil {
		// (3) Mark model call as streaming on first payload upsert.
		modelCall := apiconv.NewModelCall()
		modelCall.SetMessageID(msgID)
		modelCall.SetStatus("streaming")
		o.client.PatchModelCall(ctx, modelCall)
	}

	// (4) Append delta to current body and upsert full payload.
	next := append(cur, data...)
	if _, err := o.upsertInlinePayload(ctx, id, "model_stream", "text/plain", next); err != nil {
		return fmt.Errorf("failed to update model stream: %w", err)
	}
	// (5) Link stream payload to model call once.
	if !o.streamLinked {
		if strings.TrimSpace(msgID) != "" {
			upd := apiconv.NewModelCall()
			upd.SetMessageID(msgID)
			upd.SetStreamPayloadID(id)
			if err := o.client.PatchModelCall(ctx, upd); err != nil {
				return fmt.Errorf("failed to update model payload: %w", err)
			}
			o.streamLinked = true
		}
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
	if err := o.client.PatchConversations(ctx, convw.NewConversationStatus(turn.ConversationID, "thinking")); err != nil {
		errorf("beginModelCall patch conversation status error convo=%q err=%v", strings.TrimSpace(turn.ConversationID), err)
		return fmt.Errorf("failed to update conversation: %w", err)
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

const streamPersistModeEnv = "AGENTLY_STREAM_PERSIST_MODE"

// streamPersistFinalOnly reports whether we should persist stream payload only once at finish.
// Accepts: "final" | "finish" | "onfinish". Default (empty/unknown) is legacy per-delta persistence.
func streamPersistFinalOnly() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(streamPersistModeEnv)))
	switch v {
	case "final", "finish", "onfinish":
		return true
	default:
		return false
	}
}
