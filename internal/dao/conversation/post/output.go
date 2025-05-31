package post

import (
	"github.com/viant/xdatly/handler/response"
	"github.com/viant/xdatly/handler/validator"
)

func (o *Output) setError(err error) {
	o.Status.Message = err.Error()
	o.Status.Status = "error"
}

type Output struct {
	response.Status `parameter:",kind=output,in=status" anonymous:"true"`
	Data            []*Conversation        `parameter:",kind=body"`
	Violations      []*validator.Violation `parameter:",kind=transient"`
}
