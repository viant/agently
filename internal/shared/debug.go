package shared

import "os"

// DebugAttachmentsEnabled reports whether attachment-related debug logging is enabled.
// It is intentionally coarse-grained and should never be used to print secrets
// or raw attachment bytes.
func DebugAttachmentsEnabled() bool {
	if os.Getenv("AGENTLY_DEBUG_ATTACHMENTS") != "" {
		return true
	}
	return os.Getenv("AGENTLY_DEBUG") != ""
}
