package write

import "embed"

//go:embed sql/*.sql
var FS embed.FS

type Input struct {
	Schedules []*Schedule `parameter:",kind=body,in=data"`

	CurSchedulesId *struct{ Values []string } `parameter:",kind=param,in=Schedules,dataType=scheduler/schedulewrite.Schedules" codec:"structql,uri=sql/cur_schedules_id.sql"`

	CurSchedule []*Schedule `parameter:",kind=view,in=CurSchedule" view:"CurSchedule" sql:"uri=sql/cur_schedule.sql"`

	CurScheduleById map[string]*Schedule
}

func (i *Input) EmbedFS() (fs *embed.FS) { return &FS }
