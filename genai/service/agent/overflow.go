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
	chunk := strings.TrimSpace(body[:limit])

	id := strings.TrimSpace(refMessageID)
	chunk += "[... omitted from " + fmt.Sprintf("%d", limit) + " to " + fmt.Sprintf("%d", size) + "]"

	// Prefix with overflow nore and next byte range hint
	return fmt.Sprintf(`overflow:true
messageId: %s
nextRange: %d-%d
hasMore: true
useToolToSeeMore: internal_message-show
content: |
%s`, id, limit, size, chunk), true
}
