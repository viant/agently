package agent

import (
	"log"
	"os"

	"github.com/viant/agently/internal/shared"
)

func debugAttachmentf(format string, args ...interface{}) {
	if !shared.DebugAttachmentsEnabled() {
		return
	}
	log.New(os.Stdout, "", log.LstdFlags).Printf("[attachments] "+format, args...)
}
