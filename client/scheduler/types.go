package scheduler

import (
	runpkg "github.com/viant/agently/pkg/agently/scheduler/run"
	runwrite "github.com/viant/agently/pkg/agently/scheduler/run/write"
	schedulepkg "github.com/viant/agently/pkg/agently/scheduler/schedule"
	schedwrite "github.com/viant/agently/pkg/agently/scheduler/schedule/write"
)

// Type aliases to reuse schedule models from the store package while
// exposing them through the scheduler client.
type (
	Schedule        = schedulepkg.ScheduleView
	Run             = runpkg.RunView
	MutableSchedule = schedwrite.Schedule
	MutableRun      = runwrite.Run
)
