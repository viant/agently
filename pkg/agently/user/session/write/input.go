package write

import "embed"

//go:embed sql/*.sql
var FS embed.FS

type Input struct {
	Session    *Session `parameter:",kind=body,in=data"`
	CurSession *Session `parameter:",kind=view,in=CurSession" view:"CurSession" sql:"uri=sql/cur_session.sql"`
}

func (i *Input) EmbedFS() (fs *embed.FS) { return &FS }
