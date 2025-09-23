package elicitation

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	apiconv "github.com/viant/agently/client/conversation"
)

// StorePayload persists an elicitation payload and links it to the elicitation message.
func StorePayload(ctx context.Context, client apiconv.Client, convID, elicitationID string, payload map[string]interface{}) error {
	msg, err := client.GetMessageByElicitation(ctx, convID, elicitationID)
	if err != nil {
		return err
	}
	if msg == nil {
		return ErrMessageNotFound
	}
	raw, _ := json.Marshal(payload)
	pid := uuid.New().String()
	p := apiconv.NewPayload()
	p.SetId(pid)
	p.SetKind("elicitation_response")
	p.SetMimeType("application/json")
	p.SetSizeBytes(len(raw))
	p.SetStorage("inline")
	p.SetInlineBody(raw)
	if err := client.PatchPayload(ctx, p); err != nil {
		return err
	}
	upd := apiconv.NewMessage()
	upd.SetId(msg.Id)
	upd.SetElicitationPayloadID(pid)
	return client.PatchMessage(ctx, upd)
}

var ErrMessageNotFound = errors.New("elicitation message not found")
