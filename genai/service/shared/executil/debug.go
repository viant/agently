package executil

import (
	"context"
	"log"
)

func debugf(ctx context.Context, format string, args ...interface{}) {
	log.Printf("[executil] "+format, args...)
}
