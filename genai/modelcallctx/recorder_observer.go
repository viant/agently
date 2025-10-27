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
	"github.com/viant/agently/genai/memory"
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
	// Optional: resolve token prices for a model (per 1k tokens).
	priceProvider TokenPriceProvider
}

func (o *recorderObserver) OnCallStart(ctx context.Context, info Info) (context.Context, error) {
	o.start = info
	o.hasBeg = true
	if info.StartedAt.IsZero() {
		o.start.StartedAt = time.Now()
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

	// Emit planner interim message if response exists
	if info.LLMResponse != nil {
		if err := o.patchInterimFlag(ctx, msgID); err != nil {
			return err
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
	return nil
}

// OnStreamDelta aggregates streamed chunks. Persisted once in FinishModelCall.
func (o *recorderObserver) OnStreamDelta(ctx context.Context, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	o.acc.Write(data)
	// Best-effort append to stream payload inline body using conversation client
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

	msgID := memory.ModelMessageIDFromContext(ctx)

	var cur []byte
	pv, err := o.client.GetPayload(ctx, id)
	if err == nil && pv != nil && pv.InlineBody != nil {
		cur = *pv.InlineBody
	}
	if pv == nil {
		modelCall := apiconv.NewModelCall()
		modelCall.SetMessageID(msgID)
		modelCall.SetStatus("streaming")
		o.client.PatchModelCall(ctx, modelCall)
	}

	next := append(cur, data...)
	if _, err := o.upsertInlinePayload(ctx, id, "model_stream", "text/plain", next); err != nil {
		return fmt.Errorf("failed to update model stream: %w", err)
	}
	// Link stream payload to model call upon first successful upsert to satisfy FK early.
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
	_, err := apiconv.AddMessage(ctx, o.client, &turn,
		apiconv.WithId(msgID),
		apiconv.WithMode(mode),
		apiconv.WithRole("assistant"),
		apiconv.WithType("text"),
		apiconv.WithCreatedByUserID(turn.Assistant),
		apiconv.WithInterim(1),
	)
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
	return o.client.PatchMessage(ctx, msg)
}

//298c12dc-d9d9-45d1-b340-09611803c940

// beginModelCall persists the initial model call and associated request payloads.
func (o *recorderObserver) beginModelCall(ctx context.Context, msgID string, turn memory.TurnMeta, info Info) error {
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
			return err
		}
		mc.SetRequestPayloadID(reqID)
	}
	// provider request snapshot
	if len(info.RequestJSON) > 0 {
		prID, err := o.upsertInlinePayload(ctx, "", "provider_request", "application/json", info.RequestJSON)
		if err != nil {
			return err
		}
		mc.SetProviderRequestPayloadID(prID)
	}
	if err := o.client.PatchConversations(ctx, convw.NewConversationStatus(turn.ConversationID, "thinking")); err != nil {
		return fmt.Errorf("failed to update conversation: %w", err)
	}
	// Do not link stream payload at start to avoid FK violation.
	// Stream payload link will be set after the payload is created (OnStreamDelta/OnCallEnd).
	return o.client.PatchModelCall(ctx, mc)
}

// finishModelCall persists final model call updates, including response payloads and usage.
func (o *recorderObserver) finishModelCall(ctx context.Context, msgID, status string, info Info, streamTxt string) error {
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
				return err
			}
			upd.SetResponsePayloadID(respID)
		}
	}
	if len(info.ResponseJSON) > 0 {
		provID, err := o.upsertInlinePayload(ctx, "", "provider_response", "application/json", []byte(info.ResponseJSON))
		if err != nil {
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
			return err
		}
		upd.SetStreamPayloadID(sid)
	}
	// usage mapping
	if info.Usage != nil {
		u := info.Usage
		if debugPricingEnabled() {
			debugPricingf("usage model=%s pt=%d ct=%d cached=%d", info.Model, u.PromptTokens, u.CompletionTokens, func() int {
				if u.CachedTokens > 0 {
					return u.CachedTokens
				}
				if u.PromptCachedTokens > 0 {
					return u.PromptCachedTokens
				}
				return 0
			}())
		}
		if u.PromptTokens > 0 {
			upd.SetPromptTokens(u.PromptTokens)
		}
		if u.CompletionTokens > 0 {
			upd.SetCompletionTokens(u.CompletionTokens)
		}
		if u.TotalTokens > 0 {
			upd.SetTotalTokens(u.TotalTokens)
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
	return o.client.PatchModelCall(ctx, upd)
}

// upsertInlinePayload creates or updates an inline payload and returns its id.
// If id is empty, a new id is generated.
func (o *recorderObserver) upsertInlinePayload(ctx context.Context, id, kind, mime string, body []byte) (string, error) {
	if strings.TrimSpace(id) == "" {
		id = uuid.New().String()
	}
	pw := apiconv.NewPayload()
	pw.SetId(id)
	pw.SetKind(kind)
	pw.SetMimeType(mime)
	pw.SetSizeBytes(len(body))
	pw.SetStorage("inline")
	pw.SetInlineBody(body)
	if err := o.client.PatchPayload(ctx, pw); err != nil {
		return "", err
	}
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
