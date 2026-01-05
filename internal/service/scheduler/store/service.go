package store

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	schstore "github.com/viant/agently/client/scheduler/store"
	"github.com/viant/agently/internal/codec"
	runpkg "github.com/viant/agently/pkg/agently/scheduler/run"
	schedulepkg "github.com/viant/agently/pkg/agently/scheduler/schedule"
	schlease "github.com/viant/agently/pkg/agently/scheduler/schedule/lease"

	runwrite "github.com/viant/agently/pkg/agently/scheduler/run/write"
	schedwrite "github.com/viant/agently/pkg/agently/scheduler/schedule/write"
	"github.com/viant/datly"
	"github.com/viant/datly/repository/contract"
)

// Service provides CRUD-style operations for schedules and runs using datly components.
type Service struct{ dao *datly.Service }

// New constructs a schedule store Service using the provided datly service and registers components.
func New(ctx context.Context, dao *datly.Service) (schstore.Client, error) {
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
	if err := runpkg.DefineRunScheduleNestedComponent(ctx, dao); err != nil {
		return err
	}
	if err := runpkg.DefineRunNowComponent(ctx, dao); err != nil {
		return err
	}
	if _, err := schedwrite.DefineComponent(ctx, dao); err != nil {
		return err
	}
	if _, err := runwrite.DefineComponent(ctx, dao); err != nil {
		return err
	}
	if _, err := schlease.DefineClaimLeaseComponent(ctx, dao); err != nil {
		return err
	}
	if _, err := schlease.DefineReleaseLeaseComponent(ctx, dao); err != nil {
		return err
	}
	return nil
}

func (s *Service) GetSchedule(ctx context.Context, id string, session ...codec.SessionOption) (*schedulepkg.ScheduleView, error) {
	if s == nil || s.dao == nil {
		return nil, nil
	}
	in := schedulepkg.ScheduleInput{Id: id, Has: &schedulepkg.ScheduleInputHas{Id: true}}
	out := &schedulepkg.ScheduleOutput{}
	uri := strings.ReplaceAll(schedulepkg.SchedulePathURI, "{id}", id)
	opts := []datly.OperateOption{datly.WithOutput(out), datly.WithURI(uri), datly.WithInput(&in)}
	if len(session) > 0 {
		opts = append(opts, datly.WithSessionOptions(session...))
	}
	if _, err := s.dao.Operate(ctx, opts...); err != nil {
		return nil, err
	}
	if len(out.Data) == 0 {
		return nil, nil
	}
	return out.Data[0], nil
}

func (s *Service) GetRuns(ctx context.Context, scheduleID, since string, session ...codec.SessionOption) ([]*runpkg.RunView, error) {
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
	opts := []datly.OperateOption{datly.WithOutput(out), datly.WithURI(uri), datly.WithInput(&in)}
	if len(session) > 0 {
		opts = append(opts, datly.WithSessionOptions(session...))
	}
	if _, err := s.dao.Operate(ctx, opts...); err != nil {
		return nil, err
	}
	return out.Data, nil
}

func (s *Service) PatchSchedule(ctx context.Context, schedule *schedwrite.Schedule) error {
	if s == nil || s.dao == nil || schedule == nil {
		return nil
	}
	out, err := s.PatchSchedules(ctx, &schedwrite.Input{Schedules: []*schedwrite.Schedule{schedule}})
	if err != nil {
		return err
	}
	if out != nil && len(out.Violations) > 0 {
		return errors.New(out.Violations[0].Message)
	}
	return nil
}

func (s *Service) PatchRun(ctx context.Context, run *runwrite.Run) error {
	if s == nil || s.dao == nil || run == nil {
		return nil
	}
	out, err := s.PatchRuns(ctx, &runwrite.Input{Runs: []*runwrite.Run{run}})
	if err != nil {
		return err
	}
	if out != nil && len(out.Violations) > 0 {
		return errors.New(out.Violations[0].Message)
	}
	return nil
}

func (s *Service) TryClaimSchedule(ctx context.Context, scheduleID, leaseOwner string, leaseUntil time.Time) (bool, error) {
	if s == nil || s.dao == nil {
		return false, nil
	}
	in := &schlease.ClaimLeaseInput{
		ScheduleID: strings.TrimSpace(scheduleID),
		LeaseOwner: strings.TrimSpace(leaseOwner),
		LeaseUntil: leaseUntil,
		Now:        time.Now().UTC(),
		Has:        &schlease.ClaimLeaseInputHas{ScheduleID: true, LeaseOwner: true, LeaseUntil: true, Now: true},
	}
	out := &schlease.ClaimLeaseOutput{}
	_, err := s.dao.Operate(ctx,
		datly.WithPath(contract.NewPath(http.MethodPost, schlease.ClaimLeasePathURI)),
		datly.WithInput(in),
		datly.WithOutput(out),
	)
	if err != nil {
		return false, err
	}
	return out.Claimed, nil
}

func (s *Service) ReleaseScheduleLease(ctx context.Context, scheduleID, leaseOwner string) (bool, error) {
	if s == nil || s.dao == nil {
		return false, nil
	}
	in := &schlease.ReleaseLeaseInput{
		ScheduleID: strings.TrimSpace(scheduleID),
		LeaseOwner: strings.TrimSpace(leaseOwner),
		Has:        &schlease.ReleaseLeaseInputHas{ScheduleID: true, LeaseOwner: true},
	}
	out := &schlease.ReleaseLeaseOutput{}
	_, err := s.dao.Operate(ctx,
		datly.WithPath(contract.NewPath(http.MethodPost, schlease.ReleaseLeasePathURI)),
		datly.WithInput(in),
		datly.WithOutput(out),
	)
	if err != nil {
		return false, err
	}
	return out.Released, nil
}

// PatchSchedules applies a batch write and returns component output (including violations).
func (s *Service) PatchSchedules(ctx context.Context, in *schedwrite.Input, extra ...datly.OperateOption) (*schedwrite.Output, error) {
	if s == nil || s.dao == nil || in == nil || len(in.Schedules) == 0 {
		return &schedwrite.Output{}, nil
	}
	out := &schedwrite.Output{}
	opts := []datly.OperateOption{datly.WithPath(contract.NewPath(http.MethodPatch, schedwrite.PathURI)), datly.WithInput(in), datly.WithOutput(out)}
	if len(extra) > 0 {
		opts = append(opts, extra...)
	}
	_, err := s.dao.Operate(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// PatchRuns applies a batch write and returns component output (including violations).
func (s *Service) PatchRuns(ctx context.Context, in *runwrite.Input, extra ...datly.OperateOption) (*runwrite.Output, error) {
	if s == nil || s.dao == nil || in == nil || len(in.Runs) == 0 {
		return &runwrite.Output{}, nil
	}
	out := &runwrite.Output{}
	opts := []datly.OperateOption{datly.WithPath(contract.NewPath(http.MethodPatch, runwrite.PathURI)), datly.WithInput(in), datly.WithOutput(out)}
	if len(extra) > 0 {
		opts = append(opts, extra...)
	}
	_, err := s.dao.Operate(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) GetSchedules(ctx context.Context, session ...codec.SessionOption) ([]*schedulepkg.ScheduleView, error) {
	if s == nil || s.dao == nil {
		return nil, nil
	}
	out := &schedulepkg.ScheduleOutput{}
	opts := []datly.OperateOption{datly.WithOutput(out), datly.WithURI(schedulepkg.SchedulePathListURI), datly.WithInput(&schedulepkg.ScheduleListInput{})}
	if len(session) > 0 {
		opts = append(opts, datly.WithSessionOptions(session...))
	}
	if _, err := s.dao.Operate(ctx, opts...); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// ReadSchedules executes the schedule list component with input and returns full component output
func (s *Service) ReadSchedules(ctx context.Context, in *schedulepkg.ScheduleListInput, session []codec.SessionOption, extra ...datly.OperateOption) (*schedulepkg.ScheduleOutput, error) {
	if s == nil || s.dao == nil {
		return nil, nil
	}
	if in == nil {
		in = &schedulepkg.ScheduleListInput{}
	}
	out := &schedulepkg.ScheduleOutput{}
	opts := []datly.OperateOption{datly.WithOutput(out), datly.WithURI(schedulepkg.SchedulePathListURI), datly.WithInput(in)}
	if len(session) > 0 {
		opts = append(opts, datly.WithSessionOptions(session...))
	}
	if len(extra) > 0 {
		opts = append(opts, extra...)
	}
	if _, err := s.dao.Operate(ctx, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

// ReadSchedule executes the schedule component with input and returns full component output
func (s *Service) ReadSchedule(ctx context.Context, in *schedulepkg.ScheduleInput, session []codec.SessionOption, extra ...datly.OperateOption) (*schedulepkg.ScheduleOutput, error) {
	if s == nil || s.dao == nil {
		return nil, nil
	}
	if in == nil {
		in = &schedulepkg.ScheduleInput{}
	}
	id := strings.TrimSpace(in.Id)
	uri := strings.ReplaceAll(schedulepkg.SchedulePathURI, "{id}", id)
	out := &schedulepkg.ScheduleOutput{}
	opts := []datly.OperateOption{datly.WithOutput(out), datly.WithURI(uri), datly.WithInput(in)}
	if len(session) > 0 {
		opts = append(opts, datly.WithSessionOptions(session...))
	}
	if len(extra) > 0 {
		opts = append(opts, extra...)
	}
	if _, err := s.dao.Operate(ctx, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

// ReadRuns executes the run list component with input and returns full component output
func (s *Service) ReadRuns(ctx context.Context, in *runpkg.RunInput, session []codec.SessionOption, extra ...datly.OperateOption) (*runpkg.RunOutput, error) {
	if s == nil || s.dao == nil {
		return nil, nil
	}
	if in == nil {
		in = &runpkg.RunInput{}
	}
	id := strings.TrimSpace(in.Id)
	uri := strings.ReplaceAll(runpkg.RunPathURI, "{id}", id)
	out := &runpkg.RunOutput{}
	opts := []datly.OperateOption{datly.WithOutput(out), datly.WithURI(uri), datly.WithInput(in)}
	if len(session) > 0 {
		opts = append(opts, datly.WithSessionOptions(session...))
	}
	if len(extra) > 0 {
		opts = append(opts, extra...)
	}
	if _, err := s.dao.Operate(ctx, opts...); err != nil {
		return nil, err
	}
	return out, nil
}
