package schedule

import (
	"context"
	"fmt"
	"reflect"

	"github.com/viant/datly"
	"github.com/viant/datly/repository"
	"github.com/viant/datly/repository/contract"
	"github.com/viant/datly/view"
)

type ScheduleListInput struct {
	DefaultPredicate string `parameter:",kind=const,in=value" predicate:"handler,group=0,*schedule.Filter" value:"0"`
}

const SchedulePathListURI = "/v1/api/agently/scheduler/schedule/"

func DefineScheduleListComponent(ctx context.Context, srv *datly.Service) error {
	aComponent, err := repository.NewComponent(
		contract.NewPath("GET", SchedulePathListURI),
		repository.WithResource(srv.Resource()),
		repository.WithContract(
			reflect.TypeOf(ScheduleListInput{}),
			reflect.TypeOf(ScheduleOutput{}), &ScheduleFS, view.WithConnectorRef("agently")))

	if err != nil {
		return fmt.Errorf("failed to create Schedule component: %w", err)
	}
	if err := srv.AddComponent(ctx, aComponent); err != nil {
		return fmt.Errorf("failed to add Schedule component: %w", err)
	}
	return nil
}
