package executil

import (
	"encoding/json"
	"strings"
)

// redactToolResultIfNeeded removes large binary data (for example, base64 images) from a tool result
// so the tool output does not consume the LLM context window.
//
// The redaction is intentionally scoped to tools where we persist the binary as an attachment.
func redactToolResultIfNeeded(toolName, result string) (string, bool) {
	if !isReadImageTool(toolName) {
		return "", false
	}
	redacted, ok := redactReadImageBase64(result)
	if !ok {
		return "", false
	}
	return redacted, true
}

func redactReadImageBase64(result string) (string, bool) {
	if strings.TrimSpace(result) == "" {
		return "", false
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(result), &payload); err != nil || payload == nil {
		return "", false
	}
	raw, hadKey := payload["dataBase64"]
	if !hadKey {
		return "", false
	}
	if s, ok := raw.(string); ok && strings.TrimSpace(s) == "" {
		return "", false
	}
	payload["dataBase64"] = ""
	payload["dataBase64Omitted"] = true
	b, err := json.Marshal(payload)
	if err != nil {
		return "", false
	}
	return string(b), true
}
