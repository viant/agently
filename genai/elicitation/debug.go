package elicitation

import (
	"log"
	"os"
	"sort"
	"strings"
)

// DebugEnabled reports whether conversation debug logging is enabled.
// Enable with AGENTLY_SCHEDULER_DEBUG=1 (or true/yes/on).
// Legacy env (deprecated): AGENTLY_CONVERSATION_DEBUG, AGENTLY_DEBUG_ELICITATION.
func DebugEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("AGENTLY_SCHEDULER_DEBUG"))) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

// DebugConversationEnabled kept for backward compatibility with existing calls.
func DebugConversationEnabled() bool { return DebugEnabled() }

func debugf(format string, args ...any) { infof(format, args...) }

func infof(format string, args ...any) {
	if !DebugEnabled() {
		return
	}
	log.Printf("[debug][conversation][INFO] "+format, args...)
}

func warnf(format string, args ...any) {
	if !DebugEnabled() {
		return
	}
	log.Printf("[debug][conversation][WARN] "+format, args...)
}

func errorf(format string, args ...any) {
	if !DebugEnabled() {
		return
	}
	log.Printf("[debug][conversation][ERROR] "+format, args...)
}

func headString(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func tailString(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func PayloadKeys(payload map[string]interface{}) []string {
	if len(payload) == 0 {
		return nil
	}
	out := make([]string, 0, len(payload))
	for k := range payload {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
