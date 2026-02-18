package modelcallctx

import (
	"log"
	"os"
	"strings"
)

// DebugEnabled reports whether conversation debug logging is enabled.
// Enable with AGENTLY_SCHEDULER_DEBUG=1 (or true/yes/on).
// Legacy env (deprecated): AGENTLY_CONVERSATION_DEBUG.
func DebugEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("AGENTLY_SCHEDULER_DEBUG"))) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

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
