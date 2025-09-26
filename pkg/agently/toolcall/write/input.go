package write

import "embed"

//go:embed sql/*.sql
var FS embed.FS

type Input struct {
	ToolCalls []*ToolCall `parameter:",kind=body,in=data"`

	CurIDs *struct{ Values []string } `parameter:",kind=param,in=ToolCalls,dataType=toolcall/write.ToolCalls" codec:"structql,uri=sql/cur_ids.sql"`

	Cur []*ToolCall `parameter:",kind=view,in=Cur" view:"Cur" sql:"uri=sql/cur_tool_call.sql"`

	CurByID map[string]*ToolCall
}

func (i *Input) EmbedFS() (fs *embed.FS) { return &FS }
