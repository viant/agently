package write

import "embed"

//go:embed sql/*.sql
var FS embed.FS

type Input struct {
	Usages []*Usage `parameter:",kind=body,in=data"`

	CurIds *struct{ Values []string } `parameter:",kind=param,in=Usages,dataType=usage/write.Usages" codec:"structql,uri=sql/cur_ids.sql"`

	Cur []*Usage `parameter:",kind=view,in=Cur" view:"Cur" sql:"uri=sql/cur_conversation.sql"`

	CurByID map[string]*Usage
}

func (i *Input) EmbedFS() (fs *embed.FS) { return &FS }
