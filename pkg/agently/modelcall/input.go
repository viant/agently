package modelcall

import "embed"

//go:embed sql/*.sql
var FS embed.FS

type Input struct {
	ModelCalls []*ModelCall `parameter:",kind=body,in=data"`

	CurIDs *struct{ Values []string } `parameter:",kind=param,in=ModelCalls,dataType=modelcall/write.ModelCalls" codec:"structql,uri=sql/cur_ids.sql"`

	Cur []*ModelCall `parameter:",kind=view,in=Cur" view:"Cur" sql:"uri=sql/cur_model_call.sql"`

	CurByID map[string]*ModelCall
}

func (i *Input) EmbedFS() (fs *embed.FS) { return &FS }
