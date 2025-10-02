package store

import (
	"github.com/viant/agently/pkg/agently/scheduler/run"
	runwrite "github.com/viant/agently/pkg/agently/scheduler/run/write"
	"github.com/viant/agently/pkg/agently/scheduler/schedule"
	schedwrite "github.com/viant/agently/pkg/agently/scheduler/schedule/write"
)

// Read models (aliases to generated datly views)
type Schedule = schedule.ScheduleView
type Run = run.RunView

// Mutable write models
type MutableSchedule = schedwrite.Schedule
type MutableRun = runwrite.Run
