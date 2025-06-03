package post

import (
	"embed"
	"github.com/viant/xdatly/types/core"
	"reflect"
)

//go:embed tool/*.sql
var ToolPostFS embed.FS

type Input struct {
	ToolCall []*ToolCall `parameter:",kind=body,in=data"`

	CurToolCallId *struct{ Values []int } `parameter:",kind=param,in=ToolCall,dataType=tool.ToolCall" codec:"structql,uri=tool/cur_tool_call_id.sql"`

	CurTool []*ToolCall `parameter:",kind=view,in=CurTool" view:"CurTool" sql:"uri=tool/cur_tool.sql"`

	CurToolById IndexedToolCall
}

func (i *Input) EmbedFS() (fs *embed.FS) {
	return &ToolPostFS
}
