package user

import (
	"context"
	"fmt"
	"github.com/viant/datly"
	"github.com/viant/datly/repository"
	"github.com/viant/datly/repository/contract"
	"github.com/viant/datly/view"
	"reflect"
)

const PathListURI = "/v1/api/agently/user/"

func DefineListComponent(ctx context.Context, srv *datly.Service) error {
	c, err := repository.NewComponent(
		contract.NewPath("GET", PathListURI),
		repository.WithResource(srv.Resource()),
		repository.WithContract(
			reflect.TypeOf(Input{}),
			reflect.TypeOf(Output{}), &FS, view.WithConnectorRef("agently")))
	if err != nil {
		return fmt.Errorf("failed to create UserList component: %w", err)
	}
	if err := srv.AddComponent(ctx, c); err != nil {
		return fmt.Errorf("failed to add UserList component: %w", err)
	}
	return nil
}
