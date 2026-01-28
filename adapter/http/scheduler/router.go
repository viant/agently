package scheduler

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
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

func registerRoutes(ctx context.Context, dao *datly.Service, router *datly.Router[Service]) error {
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
		return nil, err
	}

	in := input.Data
	id := strings.TrimSpace(in.Id)
	schedID := strings.TrimSpace(in.ScheduleId)
	// For {id} alias, schedule id may come via Id if ScheduleId absent
	if schedID == "" {
		schedID = id
	}
	if schedID == "" {
		return nil, fmt.Errorf("scheduleId is required")
	}

	now := time.Now().UTC()
	runID := id
	if runID == "" {
		runID = uuid.NewString()
	}
	status := strings.TrimSpace(in.Status)
	if status == "" {
		status = "pending"
	}

	// Always enqueue a run record so serverless deployments can rely on a dedicated runner.
	run := &runwrite.Run{}
	run.SetId(runID)
	run.SetScheduleId(schedID)
	run.SetStatus(status)
	if strings.TrimSpace(in.ConversationKind) != "" {
		run.SetConversationKind(strings.TrimSpace(in.ConversationKind))
	} else {
		run.SetConversationKind("scheduled")
	}
	run.SetScheduledFor(now)
	run.SetCreatedAt(now)
	if in.ConversationId != nil && strings.TrimSpace(*in.ConversationId) != "" {
		run.SetConversationId(strings.TrimSpace(*in.ConversationId))
	}

	if _, err := svc.store.PatchRuns(ctx, &runwrite.Input{Runs: []*runwrite.Run{run}}); err != nil {
		return nil, err
	}

	// Mark schedule as due to wake up the background runner quickly (next_run_at <= now).
	// Only do this when no in-process orchestration scheduler is wired; otherwise
	// it can cause duplicate executions when a watchdog/runner is also running.
	if svc.scheduler == nil {
		upd := &schedwrite.Schedule{}
		upd.SetId(schedID)
		upd.SetNextRunAt(now)
		_ = svc.store.PatchSchedule(ctx, upd)
	}

	// If an orchestration scheduler is wired, execute immediately (preserves legacy behaviour).
	if svc.scheduler != nil {
		mr := schapi.MutableRun{}
		mr.SetId(runID)
		mr.SetScheduleId(schedID)
		mr.SetStatus(status)
		mr.SetConversationKind(run.ConversationKind)
		mr.SetScheduledFor(now)
		if in.ConversationId != nil && strings.TrimSpace(*in.ConversationId) != "" {
			mr.SetConversationId(strings.TrimSpace(*in.ConversationId))
		}
		if err := svc.scheduler.Run(ctx, &mr); err != nil {
			return nil, err
		}
		out := &runpkg.RunNowOutput{RunId: runID}
		if mr.ConversationId != nil {
			out.ConversationId = strings.TrimSpace(*mr.ConversationId)
		}
		return out, nil
	}

	out := &runpkg.RunNowOutput{RunId: runID}
	if run.ConversationId != nil {
		out.ConversationId = strings.TrimSpace(*run.ConversationId)
	}
	return out, nil
}
