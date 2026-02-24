package run

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/viant/datly"
	"github.com/viant/datly/repository"
	"github.com/viant/datly/repository/contract"
	"github.com/viant/datly/view"
	"github.com/viant/xdatly/types/core"
	checksum "github.com/viant/xdatly/types/custom/dependency/checksum"
)

func init() {
	core.RegisterType("run", "RunDueInput", reflect.TypeOf(RunDueInput{}), checksum.GeneratedTime)
}

// RunDueInput is an internal scheduler-only read contract that intentionally
// omits the default visibility predicate handler used by RunInput.
type RunDueInput struct {
	Id              string          `parameter:",kind=path,in=id" predicate:"equal,group=0,t,schedule_id"`
	Since           string          `parameter:",kind=query,in=since" predicate:"expr,group=1,created_at >= (SELECT created_at FROM turn WHERE id = ?)"`
	ScheduledFor    time.Time       `parameter:",kind=query,in=scheduled_for" predicate:"equal,group=0,t,scheduled_for"`
	ExcludeStatuses []string        `parameter:",kind=query,in=status" predicate:"not_in,group=0,t,status"`
	Has             *RunDueInputHas `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

type RunDueInputHas struct {
	Id              bool
	Since           bool
	ScheduledFor    bool
	ExcludeStatuses bool
}

const RunPathRunDueURI = "/v1/api/agently/scheduler/run/_internal/rundue/{id}"

func DefineRunDueComponent(ctx context.Context, srv *datly.Service) error {
	aComponent, err := repository.NewComponent(
		contract.NewPath("GET", RunPathRunDueURI),
		repository.WithResource(srv.Resource()),
		repository.WithContract(
			reflect.TypeOf(RunDueInput{}),
			reflect.TypeOf(RunOutput{}), &RunFS, view.WithConnectorRef("agently")))
	if err != nil {
		return fmt.Errorf("failed to create RunDue component: %w", err)
	}
	if err := srv.AddComponent(ctx, aComponent); err != nil {
		return fmt.Errorf("failed to add RunDue component: %w", err)
	}
	return nil
}
