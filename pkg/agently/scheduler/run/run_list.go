package run

import (
	"context"
	"fmt"
	"reflect"

	"github.com/viant/datly"
	"github.com/viant/datly/repository"
	"github.com/viant/datly/repository/contract"
	"github.com/viant/datly/view"
)

var RunPathListURI = "/v1/api/agently/scheduler/run/"

func DefineRunListComponent(ctx context.Context, srv *datly.Service) error {
	aComponent, err := repository.NewComponent(
		contract.NewPath("GET", RunPathListURI),
		repository.WithResource(srv.Resource()),
		repository.WithContract(
			reflect.TypeOf(RunInput{}),
			reflect.TypeOf(RunOutput{}), &RunFS, view.WithConnectorRef("agently")))

	if err != nil {
		return fmt.Errorf("failed to create Run component: %w", err)
	}
	if err := srv.AddComponent(ctx, aComponent); err != nil {
		return fmt.Errorf("failed to add Run component: %w", err)
	}
	return nil
}
