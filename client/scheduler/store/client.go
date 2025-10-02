package store

import "context"

type Client interface {
	// Reads
	GetSchedules(ctx context.Context) ([]*Schedule, error)
	GetSchedule(ctx context.Context, id string) (*Schedule, error)
	GetRuns(ctx context.Context, scheduleID string, since string) ([]*Run, error)

	// Writes
	PatchSchedule(ctx context.Context, schedule *MutableSchedule) error
	PatchRun(ctx context.Context, run *MutableRun) error
}
