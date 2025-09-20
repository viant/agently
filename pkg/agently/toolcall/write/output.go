package write

import (
	"github.com/viant/xdatly/handler/response"
	"github.com/viant/xdatly/handler/validator"
)

type Output struct {
	response.Status `parameter:",kind=output,in=status" anonymous:"true"`
	Data            []*ToolCall            `parameter:",kind=body"`
	Violations      []*validator.Violation `parameter:",kind=transient"`
}

func (o *Output) setError(err error) { o.Status.Message = err.Error(); o.Status.Status = "error" }
