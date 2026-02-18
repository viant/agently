package executil

import (
	"context"
	"log"
	"os"
	"strings"

	"github.com/viant/agently/internal/shared"
)

func debugf(ctx context.Context, format string, args ...interface{}) {
	if !shared.DebugAttachmentsEnabled() {
		return
	}
	// Use stdout so CLI/stdout captures include debug lines.
	// Do not rely on global log output configuration.
	log.New(os.Stdout, "", log.LstdFlags).Printf("[executil] "+format, args...)
}

func debugConvEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("AGENTLY_SCHEDULER_DEBUG"))) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func debugConvf(format string, args ...interface{}) { infoConvf(format, args...) }

func infoConvf(format string, args ...interface{}) {
	if !debugConvEnabled() {
		return
	}
	log.Printf("[debug][conversation][INFO] "+format, args...)
}

func warnConvf(format string, args ...interface{}) {
	if !debugConvEnabled() {
		return
	}
	log.Printf("[debug][conversation][WARN] "+format, args...)
}

func errorConvf(format string, args ...interface{}) {
	if !debugConvEnabled() {
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
