package scheduler

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	chatcli "github.com/viant/agently/client/chat"
	agconversation "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/conversation"
	core "github.com/viant/agently/genai/service/core"
	"github.com/viant/agently/genai/tool"
	approval "github.com/viant/agently/internal/approval"
	"github.com/viant/agently/internal/codec"
	runpkg "github.com/viant/agently/pkg/agently/scheduler/run"
	runwrite "github.com/viant/agently/pkg/agently/scheduler/run/write"
	schedulelist "github.com/viant/agently/pkg/agently/scheduler/schedule"
	schedulepkg "github.com/viant/agently/pkg/agently/scheduler/schedule"
	schedwrite "github.com/viant/agently/pkg/agently/scheduler/schedule/write"
	datly "github.com/viant/datly"
)

type fakeScheduleStore struct {
	schedules []*schedulepkg.ScheduleView
	schedule  map[string]*schedulepkg.ScheduleView
	runs      map[string][]*runpkg.RunView

	claimResultByScheduleID map[string]bool
	claimedByScheduleID     map[string]struct{}
	releasedByScheduleID    map[string]struct{}

	patchedRuns      []*runwrite.Run
	patchedSchedules []*schedwrite.Schedule
}

func (f *fakeScheduleStore) GetSchedules(_ context.Context, _ ...codec.SessionOption) ([]*schedulepkg.ScheduleView, error) {
	return f.schedules, nil
}

func (f *fakeScheduleStore) GetSchedule(_ context.Context, id string, _ ...codec.SessionOption) (*schedulepkg.ScheduleView, error) {
	if f.schedule == nil {
		return nil, nil
	}
	return f.schedule[id], nil
}

func (f *fakeScheduleStore) GetRuns(_ context.Context, scheduleID string, _ string, _ ...codec.SessionOption) ([]*runpkg.RunView, error) {
	if f.runs == nil {
		return nil, nil
	}
	return f.runs[scheduleID], nil
}

func (f *fakeScheduleStore) ReadSchedules(_ context.Context, _ *schedulelist.ScheduleListInput, _ []codec.SessionOption, _ ...datly.OperateOption) (*schedulelist.ScheduleOutput, error) {
	return &schedulelist.ScheduleOutput{Data: f.schedules}, nil
}

func (f *fakeScheduleStore) ReadSchedule(_ context.Context, in *schedulelist.ScheduleInput, _ []codec.SessionOption, _ ...datly.OperateOption) (*schedulelist.ScheduleOutput, error) {
	if in == nil {
		return &schedulelist.ScheduleOutput{}, nil
	}
	row := (*schedulepkg.ScheduleView)(nil)
	if f.schedule != nil {
		row = f.schedule[in.Id]
	}
	if row == nil {
		return &schedulelist.ScheduleOutput{}, nil
	}
	return &schedulelist.ScheduleOutput{Data: []*schedulepkg.ScheduleView{row}}, nil
}

func (f *fakeScheduleStore) ReadRuns(_ context.Context, in *runpkg.RunInput, _ []codec.SessionOption, _ ...datly.OperateOption) (*runpkg.RunOutput, error) {
	if in == nil {
		return &runpkg.RunOutput{}, nil
	}
	data := f.runs[in.Id]
	return &runpkg.RunOutput{Data: data}, nil
}

func (f *fakeScheduleStore) PatchSchedules(ctx context.Context, in *schedwrite.Input, _ ...datly.OperateOption) (*schedwrite.Output, error) {
	if in == nil {
		return &schedwrite.Output{}, nil
	}
	for _, s := range in.Schedules {
		_ = f.PatchSchedule(ctx, s)
	}
	return &schedwrite.Output{Data: in.Schedules}, nil
}

func (f *fakeScheduleStore) PatchRuns(ctx context.Context, in *runwrite.Input, _ ...datly.OperateOption) (*runwrite.Output, error) {
	if in == nil {
		return &runwrite.Output{}, nil
	}
	for _, r := range in.Runs {
		_ = f.PatchRun(ctx, r)
	}
	return &runwrite.Output{Data: in.Runs}, nil
}

func (f *fakeScheduleStore) PatchSchedule(_ context.Context, schedule *schedwrite.Schedule) error {
	if schedule == nil {
		return nil
	}
	f.patchedSchedules = append(f.patchedSchedules, schedule)
	return nil
}

func (f *fakeScheduleStore) PatchRun(_ context.Context, run *runwrite.Run) error {
	if run == nil {
		return nil
	}
	f.patchedRuns = append(f.patchedRuns, run)
	return nil
}

func (f *fakeScheduleStore) TryClaimSchedule(_ context.Context, scheduleID, _ string, _ time.Time) (bool, error) {
	if f.claimedByScheduleID == nil {
		f.claimedByScheduleID = map[string]struct{}{}
	}
	f.claimedByScheduleID[scheduleID] = struct{}{}
	if f.claimResultByScheduleID == nil {
		return true, nil
	}
	v, ok := f.claimResultByScheduleID[scheduleID]
	if !ok {
		return true, nil
	}
	return v, nil
}

func (f *fakeScheduleStore) ReleaseScheduleLease(_ context.Context, scheduleID, _ string) (bool, error) {
	if f.releasedByScheduleID == nil {
		f.releasedByScheduleID = map[string]struct{}{}
	}
	f.releasedByScheduleID[scheduleID] = struct{}{}
	return true, nil
}

type fakeChat struct {
	createdConversationIDs []string
	posted                 []string
}

func (f *fakeChat) AttachManager(_ *conversation.Manager, _ *tool.Policy) {}
func (f *fakeChat) AttachCore(_ *core.Service)                            {}
func (f *fakeChat) AttachApproval(_ approval.Service)                     {}
func (f *fakeChat) Get(context.Context, chatcli.GetRequest) (*chatcli.GetResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeChat) PreflightPost(context.Context, string, chatcli.PostRequest) error { return nil }
func (f *fakeChat) Post(_ context.Context, _ string, req chatcli.PostRequest) (string, error) {
	f.posted = append(f.posted, req.Content)
	return "msg", nil
}
func (f *fakeChat) Cancel(string) bool     { return false }
func (f *fakeChat) CancelTurn(string) bool { return false }
func (f *fakeChat) CreateConversation(_ context.Context, _ chatcli.CreateConversationRequest) (*chatcli.CreateConversationResponse, error) {
	id := fmt.Sprintf("conv-%d", len(f.createdConversationIDs)+1)
	f.createdConversationIDs = append(f.createdConversationIDs, id)
	return &chatcli.CreateConversationResponse{ID: id}, nil
}
func (f *fakeChat) GetConversation(context.Context, string) (*chatcli.ConversationSummary, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeChat) ListConversations(context.Context, *agconversation.Input) ([]chatcli.ConversationSummary, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeChat) DeleteConversation(context.Context, string) error {
	return fmt.Errorf("not implemented")
}
func (f *fakeChat) Approve(context.Context, string, string, string) error {
	return fmt.Errorf("not implemented")
}
func (f *fakeChat) Elicit(context.Context, string, string, map[string]interface{}) error {
	return fmt.Errorf("not implemented")
}
func (f *fakeChat) GetPayload(context.Context, string) ([]byte, string, error) {
	return nil, "", fmt.Errorf("not implemented")
}
func (f *fakeChat) SetTurnStatus(context.Context, string, string, ...string) error {
	return fmt.Errorf("not implemented")
}
func (f *fakeChat) SetMessageStatus(context.Context, string, string) error {
	return fmt.Errorf("not implemented")
}
func (f *fakeChat) SetLastAssistentMessageStatus(context.Context, string, string) error {
	return fmt.Errorf("not implemented")
}
func (f *fakeChat) Generate(context.Context, *chatcli.GenerateInput) (*chatcli.GenerateOutput, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeChat) Query(context.Context, *chatcli.QueryInput) (*chatcli.QueryOutput, error) {
	return nil, fmt.Errorf("not implemented")
}

func TestService_RunDue_LeaseAndPending_DataDriven(t *testing.T) {
	now := time.Now().UTC()
	dueAt := now.Add(-1 * time.Minute).UTC()

	type testCase struct {
		name                  string
		runs                  []*runpkg.RunView
		claim                 bool
		expectPatchedRuns     int
		expectRunID           string
		expectRunStatus       string
		expectScheduledForUTC time.Time
	}

	pendingMatching := &runpkg.RunView{
		Id:           "run-pending-1",
		ScheduleId:   "sch-1",
		Status:       "pending",
		ScheduledFor: &dueAt,
	}
	activeRunning := &runpkg.RunView{
		Id:         "run-running-1",
		ScheduleId: "sch-1",
		Status:     "running",
	}

	testCases := []testCase{
		{
			name:                  "uses_pending_run_when_present",
			runs:                  []*runpkg.RunView{pendingMatching},
			claim:                 true,
			expectPatchedRuns:     2,
			expectRunID:           "run-pending-1",
			expectRunStatus:       "running",
			expectScheduledForUTC: dueAt,
		},
		{
			name:                  "creates_new_run_when_no_pending",
			runs:                  nil,
			claim:                 true,
			expectPatchedRuns:     2,
			expectRunID:           "",
			expectRunStatus:       "running",
			expectScheduledForUTC: dueAt,
		},
		{
			name:                  "skips_when_not_claimed",
			runs:                  []*runpkg.RunView{pendingMatching},
			claim:                 false,
			expectPatchedRuns:     0,
			expectRunID:           "",
			expectRunStatus:       "",
			expectScheduledForUTC: dueAt,
		},
		{
			name:                  "skips_when_active_running",
			runs:                  []*runpkg.RunView{activeRunning},
			claim:                 true,
			expectPatchedRuns:     0,
			expectRunID:           "",
			expectRunStatus:       "",
			expectScheduledForUTC: dueAt,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeScheduleStore{
				schedule: map[string]*schedulepkg.ScheduleView{},
				runs: map[string][]*runpkg.RunView{
					"sch-1": tc.runs,
				},
				claimResultByScheduleID: map[string]bool{"sch-1": tc.claim},
			}
			schedule := &schedulepkg.ScheduleView{
				Id:           "sch-1",
				Name:         "s",
				AgentRef:     "agent",
				Enabled:      true,
				ScheduleType: "cron",
				CronExpr:     strPtr("* * * * *"),
				Timezone:     "UTC",
				NextRunAt:    &dueAt,
				TaskPrompt:   strPtr("do"),
				CreatedAt:    dueAt.Add(-time.Hour),
			}
			store.schedules = []*schedulepkg.ScheduleView{schedule}
			store.schedule["sch-1"] = schedule

			chat := &fakeChat{}
			svc := &Service{
				sch:        store,
				chat:       chat,
				leaseOwner: "owner-1",
				leaseTTL:   60 * time.Second,
			}

			started, err := svc.RunDue(context.Background())
			assert.EqualValues(t, nil, err)
			if tc.expectPatchedRuns > 0 {
				assert.EqualValues(t, 1, started)
			} else {
				assert.EqualValues(t, 0, started)
			}

			assert.EqualValues(t, tc.expectPatchedRuns, len(store.patchedRuns))
			if tc.expectPatchedRuns == 0 {
				return
			}

			got := store.patchedRuns[len(store.patchedRuns)-1]
			if tc.expectRunID != "" {
				assert.EqualValues(t, tc.expectRunID, got.Id)
			} else {
				assert.EqualValues(t, true, got.Id != "")
			}
			assert.EqualValues(t, tc.expectRunStatus, got.Status)
			assert.EqualValues(t, true, got.ScheduledFor != nil)
			if got.ScheduledFor != nil {
				assert.EqualValues(t, tc.expectScheduledForUTC, got.ScheduledFor.UTC())
			}
		})
	}
}

func TestService_RunDue_CompletedRunForSlot_AdvancesScheduleAndSkips(t *testing.T) {
	now := time.Now().UTC()
	dueAt := now.Add(-1 * time.Minute).UTC()
	completedAt := dueAt.Add(10 * time.Second).UTC()

	completedMatching := &runpkg.RunView{
		Id:           "run-completed-1",
		ScheduleId:   "sch-1",
		Status:       "succeeded",
		ScheduledFor: &dueAt,
		CompletedAt:  &completedAt,
	}

	store := &fakeScheduleStore{
		schedule: map[string]*schedulepkg.ScheduleView{},
		runs: map[string][]*runpkg.RunView{
			"sch-1": {completedMatching},
		},
		claimResultByScheduleID: map[string]bool{"sch-1": true},
	}
	schedule := &schedulepkg.ScheduleView{
		Id:           "sch-1",
		Name:         "s",
		AgentRef:     "agent",
		Enabled:      true,
		ScheduleType: "cron",
		CronExpr:     strPtr("* * * * *"),
		Timezone:     "UTC",
		NextRunAt:    &dueAt,
		TaskPrompt:   strPtr("do"),
		CreatedAt:    dueAt.Add(-time.Hour),
	}
	store.schedules = []*schedulepkg.ScheduleView{schedule}
	store.schedule["sch-1"] = schedule

	chat := &fakeChat{}
	svc := &Service{
		sch:        store,
		chat:       chat,
		leaseOwner: "owner-1",
		leaseTTL:   60 * time.Second,
	}

	started, err := svc.RunDue(context.Background())
	assert.NoError(t, err)
	assert.EqualValues(t, 0, started)
	assert.EqualValues(t, 0, len(store.patchedRuns))
	assert.EqualValues(t, 1, len(store.patchedSchedules))
	got := store.patchedSchedules[0]
	assert.EqualValues(t, "sch-1", got.Id)
	assert.True(t, got.LastRunAt != nil)
	assert.True(t, got.NextRunAt != nil)
	assert.True(t, got.Has != nil && got.Has.LastRunAt)
	assert.True(t, got.Has != nil && got.Has.NextRunAt)
}

func TestService_RunDue_CronWithoutNextOrLastRun_Triggers(t *testing.T) {
	createdAt := time.Now().UTC().Add(-10 * time.Minute)

	store := &fakeScheduleStore{
		schedule: map[string]*schedulepkg.ScheduleView{},
	}
	schedule := &schedulepkg.ScheduleView{
		Id:           "sch-1",
		Name:         "s",
		AgentRef:     "agent",
		Enabled:      true,
		ScheduleType: "cron",
		CronExpr:     strPtr("*/1 * * * *"),
		Timezone:     "UTC",
		TaskPrompt:   strPtr("do"),
		CreatedAt:    createdAt,
	}
	store.schedules = []*schedulepkg.ScheduleView{schedule}
	store.schedule["sch-1"] = schedule

	chat := &fakeChat{}
	svc := &Service{
		sch:        store,
		chat:       chat,
		leaseOwner: "owner-1",
		leaseTTL:   60 * time.Second,
	}

	started, err := svc.RunDue(context.Background())
	assert.NoError(t, err)
	assert.EqualValues(t, 1, started)
	assert.EqualValues(t, 2, len(store.patchedRuns))
	assert.EqualValues(t, "running", store.patchedRuns[len(store.patchedRuns)-1].Status)
	assert.EqualValues(t, 1, len(store.patchedSchedules))
}

func TestService_RunDue_AdhocClearsNextRunAt(t *testing.T) {
	now := time.Now().UTC()
	dueAt := now.Add(-1 * time.Minute).UTC()

	store := &fakeScheduleStore{
		schedule: map[string]*schedulepkg.ScheduleView{},
	}
	schedule := &schedulepkg.ScheduleView{
		Id:           "sch-1",
		Name:         "s",
		AgentRef:     "agent",
		Enabled:      true,
		ScheduleType: "adhoc",
		Timezone:     "UTC",
		NextRunAt:    &dueAt,
		TaskPrompt:   strPtr("do"),
		CreatedAt:    dueAt.Add(-time.Hour),
	}
	store.schedules = []*schedulepkg.ScheduleView{schedule}
	store.schedule["sch-1"] = schedule

	chat := &fakeChat{}
	svc := &Service{
		sch:        store,
		chat:       chat,
		leaseOwner: "owner-1",
		leaseTTL:   60 * time.Second,
	}

	started, err := svc.RunDue(context.Background())
	assert.NoError(t, err)
	assert.EqualValues(t, 1, started)
	assert.EqualValues(t, 1, len(store.patchedSchedules))
	got := store.patchedSchedules[0]
	assert.EqualValues(t, "sch-1", got.Id)
	assert.True(t, got.LastRunAt != nil)
	assert.True(t, got.NextRunAt == nil)
	assert.True(t, got.Has != nil && got.Has.NextRunAt)
}

func TestService_RunDue_AdhocRunning_ClearsNextRunAtAndSkips(t *testing.T) {
	now := time.Now().UTC()
	dueAt := now.Add(-1 * time.Minute).UTC()

	activeRunning := &runpkg.RunView{
		Id:         "run-running-1",
		ScheduleId: "sch-1",
		Status:     "running",
	}

	store := &fakeScheduleStore{
		schedule: map[string]*schedulepkg.ScheduleView{},
		runs: map[string][]*runpkg.RunView{
			"sch-1": {activeRunning},
		},
		claimResultByScheduleID: map[string]bool{"sch-1": true},
	}
	schedule := &schedulepkg.ScheduleView{
		Id:           "sch-1",
		Name:         "s",
		AgentRef:     "agent",
		Enabled:      true,
		ScheduleType: "adhoc",
		Timezone:     "UTC",
		NextRunAt:    &dueAt,
		TaskPrompt:   strPtr("do"),
		CreatedAt:    dueAt.Add(-time.Hour),
	}
	store.schedules = []*schedulepkg.ScheduleView{schedule}
	store.schedule["sch-1"] = schedule

	chat := &fakeChat{}
	svc := &Service{
		sch:        store,
		chat:       chat,
		leaseOwner: "owner-1",
		leaseTTL:   60 * time.Second,
	}

	started, err := svc.RunDue(context.Background())
	assert.NoError(t, err)
	assert.EqualValues(t, 0, started)
	assert.EqualValues(t, 0, len(store.patchedRuns))
	assert.EqualValues(t, 1, len(store.patchedSchedules))
	got := store.patchedSchedules[0]
	assert.EqualValues(t, "sch-1", got.Id)
	assert.True(t, got.NextRunAt == nil)
	assert.True(t, got.Has != nil && got.Has.NextRunAt)
}

func strPtr(v string) *string { return &v }
