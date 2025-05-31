package post

import (
	"github.com/viant/agently/pkg/dependency/checksum"
	"github.com/viant/xdatly/handler/response"
	"github.com/viant/xdatly/handler/validator"
	"github.com/viant/xdatly/types/core"
	"reflect"
)

func init() {
	core.RegisterType(PackageName, "Output", reflect.TypeOf(Output{}), checksum.GeneratedTime)
}

func (o *Output) setError(err error) {
	o.Status.Message = err.Error()
	o.Status.Status = "error"
}

type Output struct {
	response.Status `parameter:",kind=output,in=status" anonymous:"true"`
	Data            []*ToolCall            `parameter:",kind=body"`
	Violations      []*validator.Violation `parameter:",kind=transient"`
}
