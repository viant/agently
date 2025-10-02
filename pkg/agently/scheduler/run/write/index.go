package write

import (
	"context"
	"github.com/viant/datly"
	"github.com/viant/datly/repository"
	"github.com/viant/datly/repository/contract"
	"reflect"
)

var PathURI = "/v1/api/agently/schedule-run"

func DefineComponent(ctx context.Context, srv *datly.Service) (*repository.Component, error) {
	return srv.AddHandler(ctx, contract.NewPath("PATCH", PathURI), &Handler{},
		repository.WithResource(srv.Resource()),
		repository.WithContract(
			reflect.TypeOf(&Input{}),
			reflect.TypeOf(&Output{}),
			&FS))
}
