package executil

import (
	"context"
	"log"
	"os"

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
