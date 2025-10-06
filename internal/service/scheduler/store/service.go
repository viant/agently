package store

import (
	"context"
	"errors"
	"net/http"
	"strings"

	schcli "github.com/viant/agently/client/scheduler/store"
	runpkg "github.com/viant/agently/pkg/agently/scheduler/run"
	schedulepkg "github.com/viant/agently/pkg/agently/scheduler/schedule"

	runwrite "github.com/viant/agently/pkg/agently/scheduler/run/write"
	schedwrite "github.com/viant/agently/pkg/agently/scheduler/schedule/write"
	"github.com/viant/datly"
	"github.com/viant/datly/repository/contract"
)

// Service provides CRUD-style operations for schedules and runs using datly components.
type Service struct{ dao *datly.Service }

// New constructs a schedule store Service using the provided datly service and registers components.
func New(ctx context.Context, dao *datly.Service) (schcli.Client, error) {
	if dao == nil {
		return nil, nil
	}
	s := &Service{dao: dao}
	if err := s.init(ctx, dao); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Service) init(ctx context.Context, dao *datly.Service) error {
	if err := schedulepkg.DefineScheduleComponent(ctx, dao); err != nil {
		return err
	}
	if err := schedulepkg.DefineScheduleListComponent(ctx, dao); err != nil {
		return err
	}
	if err := runpkg.DefineRunComponent(ctx, dao); err != nil {
		return err
	}
	if err := runpkg.DefineRunListComponent(ctx, dao); err != nil {
		return err
	}
	if _, err := schedwrite.DefineComponent(ctx, dao); err != nil {
		return err
	}
	if _, err := runwrite.DefineComponent(ctx, dao); err != nil {
		return err
	}
	return nil
}

func (s *Service) GetSchedule(ctx context.Context, id string) (*schcli.Schedule, error) {
	if s == nil || s.dao == nil {
		return nil, nil
	}
	in := schedulepkg.ScheduleInput{Id: id, Has: &schedulepkg.ScheduleInputHas{Id: true}}
	out := &schedulepkg.ScheduleOutput{}
	uri := strings.ReplaceAll(schedulepkg.SchedulePathURI, "{id}", id)
	if _, err := s.dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(uri), datly.WithInput(&in)); err != nil {
		return nil, err
	}
	if len(out.Data) == 0 {
		return nil, nil
	}
	res := schcli.Schedule(*out.Data[0])
	return &res, nil
}

func (s *Service) GetRuns(ctx context.Context, scheduleID, since string) ([]*schcli.Run, error) {
	if s == nil || s.dao == nil {
		return nil, nil
	}
	in := runpkg.RunInput{Id: scheduleID, Has: &runpkg.RunInputHas{Id: true}}
	if strings.TrimSpace(since) != "" {
		in.Since = since
		in.Has.Since = true
	}
	out := &runpkg.RunOutput{}
	uri := strings.ReplaceAll(runpkg.RunPathURI, "{id}", scheduleID)
	if _, err := s.dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(uri), datly.WithInput(&in)); err != nil {
		return nil, err
	}
	res := make([]*schcli.Run, 0, len(out.Data))
	for _, v := range out.Data {
		if v != nil {
			vv := schcli.Run(*v)
			res = append(res, &vv)
		}
	}
	return res, nil
}

func (s *Service) PatchSchedule(ctx context.Context, schedule *schcli.MutableSchedule) error {
	if s == nil || s.dao == nil || schedule == nil {
		return nil
	}
	in := &schedwrite.Input{Schedules: []*schedwrite.Schedule{(*schedwrite.Schedule)(schedule)}}
	out := &schedwrite.Output{}
	_, err := s.dao.Operate(ctx,
		datly.WithPath(contract.NewPath(http.MethodPatch, schedwrite.PathURI)),
		datly.WithInput(in),
		datly.WithOutput(out),
	)
	if err != nil {
		return err
	}
	if len(out.Violations) > 0 {
		return errors.New(out.Violations[0].Message)
	}
	return nil
}

func (s *Service) PatchRun(ctx context.Context, run *schcli.MutableRun) error {
	if s == nil || s.dao == nil || run == nil {
		return nil
	}
	in := &runwrite.Input{Runs: []*runwrite.Run{(*runwrite.Run)(run)}}
	out := &runwrite.Output{}
	_, err := s.dao.Operate(ctx,
		datly.WithPath(contract.NewPath(http.MethodPatch, runwrite.PathURI)),
		datly.WithInput(in),
		datly.WithOutput(out),
	)
	if err != nil {
		return err
	}
	if len(out.Violations) > 0 {
		return errors.New(out.Violations[0].Message)
	}
	return nil
}

func (s *Service) GetSchedules(ctx context.Context) ([]*schcli.Schedule, error) {
	if s == nil || s.dao == nil {
		return nil, nil
	}
	out := &schedulepkg.ScheduleOutput{}
	if _, err := s.dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(schedulepkg.SchedulePathListURI), datly.WithInput(&schedulepkg.ScheduleListInput{})); err != nil {
		return nil, err
	}
	res := make([]*schcli.Schedule, 0, len(out.Data))
	for _, v := range out.Data {
		if v != nil {
			vv := schcli.Schedule(*v)
			res = append(res, &vv)
		}
	}
	return res, nil
}
