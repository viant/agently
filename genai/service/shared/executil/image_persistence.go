package executil

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/viant/afs"
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/pkg/mcpname"
)

type readImageOutput struct {
	URI        string `json:"uri"`
	EncodedURI string `json:"encodedURI,omitempty"`
	MimeType   string `json:"mimeType"`
	Base64     string `json:"dataBase64"`
	Name       string `json:"name,omitempty"`
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
	mimeType := strings.TrimSpace(payload.MimeType)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	name := strings.TrimSpace(payload.Name)
	if name == "" {
		name = "(image)"
	}
	var data []byte
	if strings.TrimSpace(payload.Base64) != "" {
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(payload.Base64))
		if err != nil {
			return fmt.Errorf("decode readImage base64: %w", err)
		}
		data = decoded
	} else {
		uri := strings.TrimSpace(payload.EncodedURI)
		if uri == "" {
			uri = strings.TrimSpace(payload.URI)
		}
		if uri == "" {
			return nil
		}
		downloaded, err := afs.New().DownloadWithURL(ctx, uri)
		if err != nil {
			return fmt.Errorf("download readImage encoded uri: %w", err)
		}
		data = downloaded
	}
	parentMsgID := resolveAttachmentParentMessageID(turn, toolMsgID)
	return addToolAttachment(ctx, conv, turn, parentMsgID, name, strings.TrimSpace(payload.URI), mimeType, data)
}

func isReadImageTool(toolName string) bool {
	can := strings.ToLower(strings.TrimSpace(mcpname.Canonical(toolName)))
	switch can {
	case "resources-readimage", "system_image-readimage":
		return true
	}
	return false
}

// resolveAttachmentParentMessageID associates tool-produced image attachments with the
// original user task message (same semantics as QueryInput attachments).
//
// For tool-produced attachments we want the stable turn-scoped user message id.
// In Agently the user task message id is the turn id, while ParentMessageID may
// drift (e.g. to tool/model messages during execution).
//
// Fall back to ParentMessageID only when TurnID is missing.
func resolveAttachmentParentMessageID(turn memory.TurnMeta, fallback string) string {
	if tid := strings.TrimSpace(turn.TurnID); tid != "" {
		return tid
	}
	if parent := strings.TrimSpace(turn.ParentMessageID); parent != "" {
		return parent
	}
	return strings.TrimSpace(fallback)
}

func addToolAttachment(ctx context.Context, conv apiconv.Client, turn memory.TurnMeta, parentMsgID, name, uri, mimeType string, data []byte) error {
	if len(data) == 0 {
		return nil
	}

	// Attachments are stored as model_request payloads because they are intended
	// to be injected into subsequent LLM prompts (and call_payload.kind enforces
	// an allowlist).
	pid, err := createInlinePayload(ctx, conv, "model_request", mimeType, data)
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

	msg, err := apiconv.AddMessage(ctx, conv, &turn,
		// Persist as a control message linked to the user task, so transcript/UI
		// and history building treat it the same way as QueryInput attachments.
		apiconv.WithRole("user"),
		apiconv.WithType("control"),
		apiconv.WithParentMessageID(parentMsgID),
		apiconv.WithContent(name),
		apiconv.WithAttachmentPayloadID(pid),
	)
	if err != nil {
		return fmt.Errorf("persist attachment message: %w", err)
	}
	debugf(ctx, "attached image %q bytes=%d mime=%s to parent=%s (attachment message=%s)", strings.TrimSpace(name), len(data), strings.TrimSpace(mimeType), strings.TrimSpace(parentMsgID), strings.TrimSpace(msg.Id))
	return nil
}
