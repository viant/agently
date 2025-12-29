package modelcallctx

import (
	"encoding/json"
	"strings"

	"github.com/viant/agently/genai/llm"
)

// RedactGenerateRequestForTranscript returns a JSON snapshot of the request
// suitable for persisting into conversation transcripts.
//
// It removes large base64 payloads from message items (for example image/PDF
// attachments) while keeping enough metadata to understand that an attachment
// was present. This avoids exploding transcript size and log views.
func RedactGenerateRequestForTranscript(req *llm.GenerateRequest) []byte {
	if req == nil {
		return nil
	}
	clone := *req
	clone.Messages = make([]llm.Message, 0, len(req.Messages))

	for _, msg := range req.Messages {
		m := msg
		if len(m.Items) > 0 {
			items := make([]llm.ContentItem, 0, len(m.Items))
			for _, item := range m.Items {
				redacted := item
				// Redact base64 payloads (binary content) from transcript snapshots.
				if redacted.Source == llm.SourceBase64 && strings.TrimSpace(redacted.Data) != "" {
					base64Len := len(redacted.Data)
					redacted.Data = ""
					if redacted.Metadata == nil {
						redacted.Metadata = map[string]interface{}{}
					}
					redacted.Metadata["dataBase64Omitted"] = true
					redacted.Metadata["base64Len"] = base64Len
				}
				items = append(items, redacted)
			}
			m.Items = items
		}
		if len(m.ContentItems) > 0 {
			items := make([]llm.ContentItem, 0, len(m.ContentItems))
			for _, item := range m.ContentItems {
				redacted := item
				if redacted.Source == llm.SourceBase64 && strings.TrimSpace(redacted.Data) != "" {
					base64Len := len(redacted.Data)
					redacted.Data = ""
					if redacted.Metadata == nil {
						redacted.Metadata = map[string]interface{}{}
					}
					redacted.Metadata["dataBase64Omitted"] = true
					redacted.Metadata["base64Len"] = base64Len
				}
				items = append(items, redacted)
			}
			m.ContentItems = items
		}
		clone.Messages = append(clone.Messages, m)
	}

	b, err := json.Marshal(&clone)
	if err != nil {
		return nil
	}
	return b
}
