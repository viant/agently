package write

import "github.com/viant/xdatly/handler/response"

type Output struct {
	response.Status
	Violations response.Violations `parameter:",kind=output,in=violations"`
	Data       []*Run              `parameter:",kind=output,in=view"`
}

func (o *Output) setError(err error) { o.Status.Error = err.Error(); o.Status.Status = "error" }
