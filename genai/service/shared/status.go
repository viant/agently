package shared

import "strings"

// NormalizeMessageStatus maps a variety of provider- or service-specific
// statuses to the constrained set that the persistence layer accepts for
// messages: ”, 'pending','accepted','rejected','cancel','open','summary',
// 'summarized','completed'. Unknown values are returned unchanged.
func NormalizeMessageStatus(in string) string {
	s := strings.ToLower(strings.TrimSpace(in))
	switch s {
	case "":
		return s
	case "pending", "accepted", "rejected", "cancel", "open", "summary", "summarized", "completed":
		return s
	// common synonyms/variants
	case "ok", "success", "succeeded", "done", "complete":
		return "completed"
	case "failed", "fail", "error", "errored":
		return "rejected"
	case "canceled", "cancelled", "abort", "aborted":
		return "cancel"
	case "running", "streaming", "thinking", "in_progress", "progress":
		return "open"
	case "draft":
		return "pending"
	default:
		return s
	}
}
