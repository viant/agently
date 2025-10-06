package write

import "embed"

//go:embed sql/*.sql
var FS embed.FS

type Input struct {
	Runs []*Run `parameter:",kind=body,in=data"`

	CurRunsId *struct{ Values []string } `parameter:",kind=param,in=Runs,dataType=scheduler/run/write.Runs" codec:"structql,uri=sql/cur_runs_id.sql"`

	CurRun []*Run `parameter:",kind=view,in=CurRun" view:"CurRun" sql:"uri=sql/cur_run.sql"`

	CurRunById map[string]*Run
}

func (i *Input) EmbedFS() (fs *embed.FS) { return &FS }
