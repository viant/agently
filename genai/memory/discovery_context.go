package memory

import (
	"context"
	"strings"
)

type discoveryModeKey struct{}

// DiscoveryMode controls how tool discovery behaves for the current request.
type DiscoveryMode struct {
	Scheduler     bool
	Strict        bool
	ScheduleID    string
	ScheduleRunID string
}

// WithDiscoveryMode stores discovery mode in context.
func WithDiscoveryMode(ctx context.Context, mode DiscoveryMode) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	mode.ScheduleID = strings.TrimSpace(mode.ScheduleID)
	mode.ScheduleRunID = strings.TrimSpace(mode.ScheduleRunID)
	return context.WithValue(ctx, discoveryModeKey{}, mode)
}

// DiscoveryModeFromContext returns discovery mode when set.
func DiscoveryModeFromContext(ctx context.Context) (DiscoveryMode, bool) {
	if ctx == nil {
		return DiscoveryMode{}, false
	}
	mode, ok := ctx.Value(discoveryModeKey{}).(DiscoveryMode)
	if !ok {
		return DiscoveryMode{}, false
	}
	return mode, true
}

// MergeDiscoveryMode merges an update into existing discovery mode and stores
// the resulting value in context.
func MergeDiscoveryMode(ctx context.Context, update DiscoveryMode) context.Context {
	base, _ := DiscoveryModeFromContext(ctx)
	base.Scheduler = base.Scheduler || update.Scheduler
	if update.Strict {
		base.Strict = true
	}
	if v := strings.TrimSpace(update.ScheduleID); v != "" {
		base.ScheduleID = v
	}
	if v := strings.TrimSpace(update.ScheduleRunID); v != "" {
		base.ScheduleRunID = v
	}
	return WithDiscoveryMode(ctx, base)
}
