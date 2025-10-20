package agent

import (
	"fmt"
	"strings"
)

// buildOverflowPreview trims body to limit and appends an omitted trailer with an optional message reference.
// Returns the preview content and whether overflow occurred.
func buildOverflowPreview(body string, limit int, refMessageID string) (string, bool) {
	body = strings.TrimSpace(body)
	if limit <= 0 || len(body) <= limit {
		return body, false
	}
	size := len(body)
	omitted := body[limit:]
	omittedLines := 0
	if len(omitted) > 0 {
		omittedLines = strings.Count(omitted, "\n") + 1
	}
	trailer := ""
	if s := strings.TrimSpace(refMessageID); s != "" {
		trailer = " to access rest use message " + s
	}
	preview := strings.TrimSpace(body[:limit]) + "\n[... omitted from " + fmt.Sprintf("%d", limit) + " to " + fmt.Sprintf("%d", size) + ", ~" + fmt.Sprintf("%d", omittedLines) + " lines]" + trailer
	return preview, true
}
