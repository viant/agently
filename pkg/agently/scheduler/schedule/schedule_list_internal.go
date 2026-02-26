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

// ScheduleRunDueListInput is used by scheduler internals to list all schedules
// without user-visibility filtering.
type ScheduleRunDueListInput struct{}

const SchedulePathListRunDueURI = "/v1/api/agently/scheduler/schedule/_internal/rundue"

func DefineScheduleRunDueListComponent(ctx context.Context, srv *datly.Service) error {
	aComponent, err := repository.NewComponent(
		contract.NewPath("GET", SchedulePathListRunDueURI),
		repository.WithResource(srv.Resource()),
		repository.WithContract(
			reflect.TypeOf(ScheduleRunDueListInput{}),
			reflect.TypeOf(ScheduleOutput{}), &ScheduleFS, view.WithConnectorRef("agently")))

	if err != nil {
		return fmt.Errorf("failed to create Schedule run-due list component: %w", err)
	}
	if err := srv.AddComponent(ctx, aComponent); err != nil {
		return fmt.Errorf("failed to add Schedule run-due list component: %w", err)
	}
	return nil
}
