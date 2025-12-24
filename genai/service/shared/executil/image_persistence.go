package executil

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/pkg/mcpname"
)

type readImageOutput struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType"`
	Base64   string `json:"dataBase64"`
	Name     string `json:"name,omitempty"`
}

func persistToolImageAttachmentIfNeeded(ctx context.Context, conv apiconv.Client, turn memory.TurnMeta, toolMsgID, toolName, result string) error {
	if conv == nil || strings.TrimSpace(toolMsgID) == "" || strings.TrimSpace(result) == "" {
		return nil
	}
	if !isReadImageTool(toolName) {
		return nil
	}
	var payload readImageOutput
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		return fmt.Errorf("decode readImage result: %w", err)
	}
	if strings.TrimSpace(payload.Base64) == "" {
		return nil
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(payload.Base64))
	if err != nil {
		return fmt.Errorf("decode readImage base64: %w", err)
	}
	mimeType := strings.TrimSpace(payload.MimeType)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	name := strings.TrimSpace(payload.Name)
	if name == "" {
		name = "(image)"
	}
	return addToolAttachment(ctx, conv, turn, toolMsgID, name, strings.TrimSpace(payload.URI), mimeType, data)
}

func isReadImageTool(toolName string) bool {
	can := strings.ToLower(strings.TrimSpace(mcpname.Canonical(toolName)))
	switch can {
	case "resources-readimage", "system_image-readimage":
		return true
	}
	return false
}

func addToolAttachment(ctx context.Context, conv apiconv.Client, turn memory.TurnMeta, parentMsgID, name, uri, mimeType string, data []byte) error {
	if len(data) == 0 {
		return nil
	}

	msg, err := apiconv.AddMessage(ctx, conv, &turn,
		apiconv.WithRole("tool"),
		apiconv.WithType("control"),
		apiconv.WithParentMessageID(parentMsgID),
		apiconv.WithContent(name),
	)
	if err != nil {
		return fmt.Errorf("persist attachment message: %w", err)
	}

	pid, err := createInlinePayload(ctx, conv, "attachment", mimeType, data)
	if err != nil {
		return fmt.Errorf("persist attachment payload: %w", err)
	}
	if strings.TrimSpace(uri) != "" {
		updPayload := apiconv.NewPayload()
		updPayload.SetId(pid)
		updPayload.SetURI(uri)
		if err := conv.PatchPayload(ctx, updPayload); err != nil {
			return fmt.Errorf("update attachment payload uri: %w", err)
		}
	}

	link := apiconv.NewMessage()
	link.SetId(msg.Id)
	link.SetAttachmentPayloadID(pid)
	if err := conv.PatchMessage(ctx, link); err != nil {
		return fmt.Errorf("link attachment payload to message: %w", err)
	}
	return nil
}
