package modelcallctx

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/memory"
)

// recorderObserver writes model-call data directly using conversation client.
type recorderObserver struct {
	client          apiconv.Client
	start           Info
	hasBeg          bool
	acc             strings.Builder
	streamPayloadID string
	streamLinked    bool
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
		if err := o.patchInterimRequestMessage(ctx, turn, msgID, info.Payload); err != nil {
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
	_, _ = memory.TurnMetaFromContext(ctx)
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
	var cur []byte
	if pv, err := o.client.GetPayload(ctx, id); err == nil && pv != nil && pv.InlineBody != nil {
		cur = *pv.InlineBody
	}
	next := append(cur, data...)
	if _, err := o.upsertInlinePayload(ctx, id, "model_stream", "text/plain", next); err != nil {
		return err
	}
	// Link stream payload to model call upon first successful upsert to satisfy FK early.
	if !o.streamLinked {
		msgID := memory.ModelMessageIDFromContext(ctx)
		if strings.TrimSpace(msgID) != "" {
			upd := apiconv.NewModelCall()
			upd.SetMessageID(msgID)
			upd.SetStreamPayloadID(id)
			if err := o.client.PatchModelCall(ctx, upd); err != nil {
				return err
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

// patchInterimRequestMessage creates an interim assistant message capturing the request payload.
func (o *recorderObserver) patchInterimRequestMessage(ctx context.Context, turn memory.TurnMeta, msgID string, payload []byte) error {
	one := 1
	msg := apiconv.NewMessage()
	msg.SetId(msgID)
	msg.SetConversationID(turn.ConversationID)
	msg.SetTurnID(turn.TurnID)
	msg.SetParentMessageID(turn.ParentMessageID)
	msg.SetRole("assistant")
	msg.SetType("text")
	msg.SetInterim(one)
	msg.Has.Content = true
	return o.client.PatchMessage(ctx, msg)
}

// patchInterimFlag marks an existing message as interim.
func (o *recorderObserver) patchInterimFlag(ctx context.Context, msgID string) error {
	interim := 1
	msg := apiconv.NewMessage()
	msg.SetId(msgID)
	msg.SetInterim(interim)
	return o.client.PatchMessage(ctx, msg)
}

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
	mc.SetStatus("queued")
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
		if u.PromptTokens > 0 {
			upd.SetPromptTokens(u.PromptTokens)
		}
		if u.CompletionTokens > 0 {
			upd.SetCompletionTokens(u.CompletionTokens)
		}
		if u.TotalTokens > 0 {
			upd.SetTotalTokens(u.TotalTokens)
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
