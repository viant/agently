package scheduler

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	schapi "github.com/viant/agently/client/scheduler"
	schstore "github.com/viant/agently/client/scheduler/store"
	runpkg "github.com/viant/agently/pkg/agently/scheduler/run"
	runwrite "github.com/viant/agently/pkg/agently/scheduler/run/write"
	schedulepkg "github.com/viant/agently/pkg/agently/scheduler/schedule"
	schedwrite "github.com/viant/agently/pkg/agently/scheduler/schedule/write"
	datly "github.com/viant/datly"
	"github.com/viant/datly/repository/contract"
	hstate "github.com/viant/xdatly/handler/state"
)

// Service bundles store and optional orchestration for router handlers.
type Service struct {
	store     schstore.Client
	scheduler schapi.Client // optional; enables run-now
}

func registerRoutes(ctx context.Context, router *datly.Router[Service]) error {
	// GET schedule list
	if err := router.Register(ctx, contract.NewPath(http.MethodGet, schedulepkg.SchedulePathListURI), func(ctx context.Context, svc Service, r *http.Request, injector hstate.Injector, extra ...datly.OperateOption) (interface{}, error) {
		in := schedulepkg.ScheduleListInput{}
		if err := injector.Bind(ctx, &in); err != nil {
			return nil, err
		}
		return svc.store.ReadSchedules(ctx, &in, nil, extra...)
	}); err != nil {
		return err
	}

	// GET schedule by id
	if err := router.Register(ctx, contract.NewPath(http.MethodGet, schedulepkg.SchedulePathURI), func(ctx context.Context, svc Service, r *http.Request, injector hstate.Injector, extra ...datly.OperateOption) (interface{}, error) {
		in := schedulepkg.ScheduleInput{}
		if err := injector.Bind(ctx, &in); err != nil {
			return nil, err
		}
		return svc.store.ReadSchedule(ctx, &in, nil, extra...)
	}); err != nil {
		return err
	}

	// GET runs for schedule id
	if err := router.Register(ctx, contract.NewPath(http.MethodGet, runpkg.RunPathURI), func(ctx context.Context, svc Service, r *http.Request, injector hstate.Injector, extra ...datly.OperateOption) (interface{}, error) {
		in := runpkg.RunInput{}
		if err := injector.Bind(ctx, &in); err != nil {
			return nil, err
		}
		return svc.store.ReadRuns(ctx, &in, nil, extra...)
	}); err != nil {
		return err
	}

	// GET runs nested under schedule/{id}/run (alias component)
	if err := router.Register(ctx, contract.NewPath(http.MethodGet, runpkg.RunPathNestedUnderScheduleURI), func(ctx context.Context, svc Service, r *http.Request, injector hstate.Injector, extra ...datly.OperateOption) (interface{}, error) {
		in := runpkg.RunInput{}
		if err := injector.Bind(ctx, &in); err != nil {
			return nil, err
		}
		return svc.store.ReadRuns(ctx, &in, nil, extra...)
	}); err != nil {
		return err
	}

	// PATCH schedule (batch)
	if err := router.Register(ctx, contract.NewPath(http.MethodPatch, schedwrite.PathURI), func(ctx context.Context, svc Service, r *http.Request, injector hstate.Injector, extra ...datly.OperateOption) (interface{}, error) {
		in := schedwrite.Input{}
		if err := injector.Bind(ctx, &in); err != nil {
			return nil, err
		}
		if len(in.Schedules) == 0 {
			return nil, fmt.Errorf("schedules payload required")
		}
		return svc.store.PatchSchedules(ctx, &in, extra...)
	}); err != nil {
		return err
	}

	// PATCH runs (batch)
	if err := router.Register(ctx, contract.NewPath(http.MethodPatch, runwrite.PathURI), func(ctx context.Context, svc Service, r *http.Request, injector hstate.Injector, extra ...datly.OperateOption) (interface{}, error) {
		in := runwrite.Input{}
		if err := injector.Bind(ctx, &in); err != nil {
			return nil, err
		}
		if len(in.Runs) == 0 {
			return nil, fmt.Errorf("runs payload required")
		}
		return svc.store.PatchRuns(ctx, &in, extra...)
	}); err != nil {
		return err
	}

	// POST run-now aliases using shared handler
	hRunNow := func(ctx context.Context, svc Service, r *http.Request, injector hstate.Injector, _ ...datly.OperateOption) (interface{}, error) {
		return handleRunNow(ctx, svc, injector)
	}
	if err := router.Register(ctx, contract.NewPath(http.MethodPost, runpkg.RunNowPathURI), hRunNow); err != nil {
		return err
	}

	return nil
}

// handleRunNow binds input via injector, prepares a MutableRun, delegates to scheduler, and returns output.
func handleRunNow(ctx context.Context, svc Service, injector hstate.Injector) (interface{}, error) {
	type Body struct {
		Data *runwrite.Run `parameter:",kind=body"`
	}
	input := &Body{Data: &runwrite.Run{}}
	if err := injector.Bind(ctx, input); err != nil {
		fmt.Println("error:", err)
		return nil, err
	}

	in := input.Data
	mr := schapi.MutableRun{}
	id := strings.TrimSpace(in.Id)
	schedID := strings.TrimSpace(in.ScheduleId)
	// For {id} alias, schedule id may come via Id if ScheduleId absent
	if schedID == "" {
		schedID = id
	}
	if schedID == "" {
		return nil, fmt.Errorf("scheduleId is required")
	}
	if id != "" {
		mr.SetId(id)
	}
	mr.SetScheduleId(schedID)
	status := strings.TrimSpace(in.Status)
	if status == "" {
		status = "pending"
	}
	mr.SetStatus(status)
	if in.ConversationId != nil && strings.TrimSpace(*in.ConversationId) != "" {
		mr.SetConversationId(strings.TrimSpace(*in.ConversationId))
	}
	if strings.TrimSpace(in.ConversationKind) != "" {
		mr.SetConversationKind(strings.TrimSpace(in.ConversationKind))
	}
	if err := svc.scheduler.Run(ctx, &mr); err != nil {
		return nil, err
	}
	out := &runpkg.RunNowOutput{RunId: mr.Id}
	if mr.ConversationId != nil {
		out.ConversationId = strings.TrimSpace(*mr.ConversationId)
	}
	return out, nil
}
