package store

import (
	"context"
	"time"

	"github.com/viant/agently/internal/codec"
	runpkg "github.com/viant/agently/pkg/agently/scheduler/run"
	runwrite "github.com/viant/agently/pkg/agently/scheduler/run/write"
	schedulepkg "github.com/viant/agently/pkg/agently/scheduler/schedule"
	schedwrite "github.com/viant/agently/pkg/agently/scheduler/schedule/write"
	datly "github.com/viant/datly"
)

type Client interface {
	// Reads
	GetSchedules(ctx context.Context, session ...codec.SessionOption) ([]*schedulepkg.ScheduleView, error)
	GetSchedule(ctx context.Context, id string, session ...codec.SessionOption) (*schedulepkg.ScheduleView, error)
	GetRuns(ctx context.Context, scheduleID string, since string, session ...codec.SessionOption) ([]*runpkg.RunView, error)

	// Component-shaped reads (input -> output)
	ReadSchedules(ctx context.Context, in *schedulepkg.ScheduleListInput, session []codec.SessionOption, extra ...datly.OperateOption) (*schedulepkg.ScheduleOutput, error)
	ReadSchedule(ctx context.Context, in *schedulepkg.ScheduleInput, session []codec.SessionOption, extra ...datly.OperateOption) (*schedulepkg.ScheduleOutput, error)
	ReadRuns(ctx context.Context, in *runpkg.RunInput, session []codec.SessionOption, extra ...datly.OperateOption) (*runpkg.RunOutput, error)

	// Writes
	PatchSchedules(ctx context.Context, in *schedwrite.Input, extra ...datly.OperateOption) (*schedwrite.Output, error)
	PatchRuns(ctx context.Context, in *runwrite.Input, extra ...datly.OperateOption) (*runwrite.Output, error)

	// Single upserts (helpers)
	PatchSchedule(ctx context.Context, schedule *schedwrite.Schedule) error
	PatchRun(ctx context.Context, run *runwrite.Run) error

	// Lease-based locking (prevents duplicate due runs across instances)
	TryClaimSchedule(ctx context.Context, scheduleID, leaseOwner string, leaseUntil time.Time) (bool, error)
	ReleaseScheduleLease(ctx context.Context, scheduleID, leaseOwner string) (bool, error)

	// Run-level lease (prevents multiple instances from finalizing the same run)
	TryClaimRun(ctx context.Context, runID, leaseOwner string, leaseUntil time.Time) (bool, error)
	ReleaseRunLease(ctx context.Context, runID, leaseOwner string) (bool, error)
}
