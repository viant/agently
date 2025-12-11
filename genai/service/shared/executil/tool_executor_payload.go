package executil

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	apiconv "github.com/viant/agently/client/conversation"
)

// persistRequestPayload stores tool request arguments and links them to the tool call.
func persistRequestPayload(ctx context.Context, conv apiconv.Client, toolMsgID string, args map[string]interface{}) (string, error) {
	b, mErr := json.Marshal(args)
	if mErr != nil {
		return "", mErr
	}
	reqID, pErr := createInlinePayload(ctx, conv, "tool_request", "application/json", b)
	if pErr != nil {
		return "", pErr
	}
	upd := apiconv.NewToolCall()
	upd.SetMessageID(toolMsgID)
	upd.RequestPayloadID = &reqID
	upd.Has.RequestPayloadID = true
	_ = conv.PatchToolCall(ctx, upd)
	return reqID, nil
}

// persistResponsePayload stores tool response content and returns the payload ID.
func persistResponsePayload(ctx context.Context, conv apiconv.Client, result string) (string, error) {
	rb := []byte(result)
	return createInlinePayload(ctx, conv, "tool_response", "text/plain", rb)
}

// createInlinePayload creates and persists an inline payload and returns its ID.
func createInlinePayload(ctx context.Context, conv apiconv.Client, kind, mime string, body []byte) (string, error) {
	pid := uuid.New().String()
	p := apiconv.NewPayload()
	p.SetId(pid)
	p.SetKind(kind)
	p.SetMimeType(mime)
	p.SetSizeBytes(len(body))
	p.SetStorage("inline")
	p.SetInlineBody(body)
	if err := conv.PatchPayload(ctx, p); err != nil {
		return "", err
	}
	return pid, nil
}
