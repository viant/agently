package run

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	schedulepkg "github.com/viant/agently/pkg/agently/scheduler/schedule"
	"github.com/viant/datly"
	"github.com/viant/datly/repository"
	"github.com/viant/datly/repository/contract"
	"github.com/viant/datly/view"
)

// RunPathNestedUnderScheduleURI exposes runs under schedule/{id}/run to reuse the same reader
// as GET /v1/api/agently/scheduler/run/{id} while providing a more intuitive nested path.
var RunPathNestedUnderScheduleURI = strings.Replace(schedulepkg.SchedulePathURI, "{id}", "{id}/run", 1)

// DefineRunScheduleNestedComponent registers an alias component for schedule/{id}/run using
// the same input/output contract as the primary Run component.
func DefineRunScheduleNestedComponent(ctx context.Context, srv *datly.Service) error {
	aComponent, err := repository.NewComponent(
		contract.NewPath("GET", RunPathNestedUnderScheduleURI),
		repository.WithResource(srv.Resource()),
		repository.WithContract(
			reflect.TypeOf(RunInput{}),
			reflect.TypeOf(RunOutput{}), &RunFS, view.WithConnectorRef("agently")))
	if err != nil {
		return fmt.Errorf("failed to create Run nested component: %w", err)
	}
	if err := srv.AddComponent(ctx, aComponent); err != nil {
		return fmt.Errorf("failed to add Run nested component: %w", err)
	}
	return nil
}
