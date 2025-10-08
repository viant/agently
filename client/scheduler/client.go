package scheduler

import "context"

import "github.com/viant/agently/internal/codec"
import runpkg "github.com/viant/agently/pkg/agently/scheduler/run"
import runwrite "github.com/viant/agently/pkg/agently/scheduler/run/write"
import schedulepkg "github.com/viant/agently/pkg/agently/scheduler/schedule"
import schedwrite "github.com/viant/agently/pkg/agently/scheduler/schedule/write"

// Type aliases to reuse schedule models from the store package while
// exposing them through the scheduler client.
type (
	Schedule        = schedulepkg.ScheduleView
	Run             = runpkg.RunView
	MutableSchedule = schedwrite.Schedule
	MutableRun      = runwrite.Run
)

// Client defines a scheduler API built on top of the schedule store.
// It provides generic, data-driven upserts for schedules and runs,
// and list/read operations.
type Client interface {
	// ListSchedules returns all schedules.
	ListSchedules(ctx context.Context, session ...codec.SessionOption) ([]*Schedule, error)

	// GetSchedule returns a schedule by id or nil if not found.
	GetSchedule(ctx context.Context, id string, session ...codec.SessionOption) (*Schedule, error)

	// Schedule creates or updates a schedule (generic upsert via Has flags).
	Schedule(ctx context.Context, in *MutableSchedule) error

	// Run creates or updates a run (generic upsert via Has flags).
	Run(ctx context.Context, in *MutableRun) error

	// GetRuns lists runs for a schedule, optionally filtered by since id.
	GetRuns(ctx context.Context, scheduleID, since string, session ...codec.SessionOption) ([]*Run, error)

	// RunDue lists all schedules, checks if they are due to run,
	// and triggers runs while avoiding duplicates. Returns number of started runs.
	RunDue(ctx context.Context) (int, error)
}
