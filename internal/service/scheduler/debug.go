package scheduler

import (
	"log"
	"os"
	"strings"
	"time"
)

// DebugEnabled reports whether scheduler debug logging is enabled.
// Enable with AGENTLY_SCHEDULER_DEBUG=1 (or true/yes/on).
func DebugEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("AGENTLY_SCHEDULER_DEBUG"))) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func debugf(format string, args ...any) {
	if !DebugEnabled() {
		return
	}
	log.Printf("[debug][scheduler] "+format, args...)
}

func redactCredRef(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	// Avoid leaking secrets in logs; keep only scheme-ish prefix and tail.
	if len(v) <= 12 {
		return "***"
	}
	head := v[:8]
	tail := v[len(v)-4:]
	return head + "***" + tail
}

func timePtrString(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}
