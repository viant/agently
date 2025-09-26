package write

import "embed"

//go:embed sql/*.sql
var FS embed.FS

type Input struct {
	Turns []*Turn `parameter:",kind=body,in=data"`

	CurTurnsId *struct{ Values []string } `parameter:",kind=param,in=Turns,dataType=turn/write.Turns" codec:"structql,uri=sql/cur_turns_id.sql"`

	CurTurn []*Turn `parameter:",kind=view,in=CurTurn" view:"CurTurn" sql:"uri=sql/cur_turn.sql"`

	CurTurnById IndexedTurn
}

func (i *Input) EmbedFS() (fs *embed.FS) { return &FS }
