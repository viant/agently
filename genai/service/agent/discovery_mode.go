package agent

import (
	"context"
	"strings"

	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/memory"
)

func applySchedulerDiscoveryMode(ctx context.Context, conv *apiconv.Conversation) context.Context {
	if conv == nil {
		return ctx
	}
	isScheduled := false
	if conv.Scheduled != nil && *conv.Scheduled == 1 {
		isScheduled = true
	}
	scheduleID := trimmedPtr(conv.ScheduleId)
	runID := trimmedPtr(conv.ScheduleRunId)
	if !isScheduled && scheduleID == "" && runID == "" {
		return ctx
	}
	return memory.MergeDiscoveryMode(ctx, memory.DiscoveryMode{
		Scheduler: true,
		// Temporary safety mode: keep scheduler discovery non-strict while
		// validating discovery diagnostics in production. Re-enable strict=true
		// after confidence that warnings/error signals are sufficient.
		Strict:        false,
		ScheduleID:    scheduleID,
		ScheduleRunID: runID,
	})
}

func trimmedPtr(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}
