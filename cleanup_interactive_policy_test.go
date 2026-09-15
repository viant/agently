package agently

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	coredata "github.com/viant/agently-core/app/store/data"
	agentlyrt "github.com/viant/agently/runtime"
)

func TestConversationCleanupPoliciesRegistersOnlyEnabledInteractivePolicy(t *testing.T) {
	data := &recordingConversationCleanupData{}
	if got := conversationCleanupPolicies(data, agentlyrt.ConversationCleanupOptions{}); len(got) != 0 {
		t.Fatalf("disabled policies = %#v", got)
	}
	policies := conversationCleanupPolicies(data, agentlyrt.ConversationCleanupOptions{
		InteractiveMode:      agentlyrt.ConversationCleanupModeDryRun,
		InteractiveRetention: 45 * 24 * time.Hour,
	})
	if len(policies) != 2 || policies[0].Name() != interactiveConversationCleanupDryRunPolicyName || policies[1].Name() != "technical_retention_interactive_dry_run" {
		t.Fatalf("enabled policies = %#v", policies)
	}
	policy, ok := policies[0].(*interactiveConversationCleanupPolicy)
	if !ok || policy.retention != 45*24*time.Hour || policy.mode != coredata.ConversationMaintenanceDryRun {
		t.Fatalf("interactive policy = %#v", policies[0])
	}

	policies = conversationCleanupPolicies(data, agentlyrt.ConversationCleanupOptions{
		InteractiveMode:      agentlyrt.ConversationCleanupModeExecute,
		InteractiveRetention: 45 * 24 * time.Hour,
	})
	if len(policies) != 2 || policies[0].Name() != interactiveConversationCleanupDeletePolicyName || policies[1].Name() != "technical_retention_interactive_delete" {
		t.Fatalf("delete policies = %#v", policies)
	}
	policy, ok = policies[0].(*interactiveConversationCleanupPolicy)
	if !ok || policy.mode != coredata.ConversationMaintenanceDelete {
		t.Fatalf("delete policy = %#v", policies[0])
	}
}

func TestInteractiveConversationCleanupPolicyUsesStableCutoffAndDryRun(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	activityAt := now.Add(-60 * 24 * time.Hour)
	data := &recordingConversationCleanupData{
		candidatePages: [][]coredata.ConversationMaintenanceCandidate{
			{{RootID: "root-1", ExpectedOwnerID: "owner-1", ActivityAt: activityAt}},
			nil,
			nil,
		},
		maintenanceResults: map[string]*coredata.ConversationMaintenanceResult{
			"root-1": {
				RootID:   "root-1",
				Kind:     coredata.ConversationMaintenanceInteractive,
				Mode:     coredata.ConversationMaintenanceDryRun,
				Eligible: true,
				Reason:   coredata.ConversationMaintenanceEligible,
			},
		},
	}
	policy := newInteractiveConversationCleanupPolicy(data, 30*24*time.Hour, false)
	policy.now = func() time.Time { return now }

	candidates, err := policy.SelectCandidates(context.Background(), conversationCleanupCursor{}, 7)
	if err != nil {
		t.Fatalf("SelectCandidates(first) error: %v", err)
	}
	if len(candidates) != 1 || candidates[0].RootID != "root-1" || candidates[0].ExpectedOwnerID != "owner-1" || !candidates[0].ActivityAt.Equal(activityAt) {
		t.Fatalf("candidates = %#v", candidates)
	}
	outcome, err := policy.ProcessCandidate(context.Background(), candidates[0])
	if err != nil {
		t.Fatalf("ProcessCandidate() error: %v", err)
	}
	if !outcome.Eligible || outcome.Deleted || outcome.Reason != string(coredata.ConversationMaintenanceEligible) {
		t.Fatalf("outcome = %#v", outcome)
	}

	now = now.Add(24 * time.Hour)
	_, err = policy.SelectCandidates(context.Background(), candidates[0].cursor(), 7)
	if err != nil {
		t.Fatalf("SelectCandidates(next page) error: %v", err)
	}
	_, err = policy.SelectCandidates(context.Background(), conversationCleanupCursor{}, 7)
	if err != nil {
		t.Fatalf("SelectCandidates(next pass) error: %v", err)
	}

	firstCutoff := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	if len(data.candidateRequests) != 3 {
		t.Fatalf("candidate requests = %#v", data.candidateRequests)
	}
	if !data.candidateRequests[0].InactiveBefore.Equal(firstCutoff) ||
		!data.candidateRequests[1].InactiveBefore.Equal(firstCutoff) ||
		!data.candidateRequests[2].InactiveBefore.Equal(firstCutoff.Add(24*time.Hour)) {
		t.Fatalf("candidate cutoffs are not stable per pass: %#v", data.candidateRequests)
	}
	firstRequest := data.candidateRequests[0]
	if firstRequest.Kind != coredata.ConversationMaintenanceInteractive || firstRequest.Limit != 7 || !firstRequest.AfterActivity.IsZero() || firstRequest.AfterRootID != "" {
		t.Fatalf("first candidate request = %#v", firstRequest)
	}
	secondRequest := data.candidateRequests[1]
	if !secondRequest.AfterActivity.Equal(activityAt) || secondRequest.AfterRootID != "root-1" {
		t.Fatalf("second candidate request = %#v", secondRequest)
	}
	if len(data.maintenanceRequests) != 1 {
		t.Fatalf("maintenance requests = %#v", data.maintenanceRequests)
	}
	maintenanceRequest := data.maintenanceRequests[0]
	if maintenanceRequest.RootID != "root-1" || maintenanceRequest.ExpectedOwnerID != "owner-1" ||
		maintenanceRequest.Kind != coredata.ConversationMaintenanceInteractive ||
		maintenanceRequest.Mode != coredata.ConversationMaintenanceDryRun ||
		!maintenanceRequest.InactiveBefore.Equal(firstCutoff) {
		t.Fatalf("maintenance request = %#v", maintenanceRequest)
	}
}

func TestInteractiveConversationCleanupWorkerDryRunAggregatesSkipReasons(t *testing.T) {
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	data := &recordingConversationCleanupData{
		candidatePages: [][]coredata.ConversationMaintenanceCandidate{
			{
				{RootID: "eligible", ExpectedOwnerID: "owner", ActivityAt: base},
				{RootID: "live", ExpectedOwnerID: "owner", ActivityAt: base.Add(time.Second)},
				{RootID: "broken", ExpectedOwnerID: "owner", ActivityAt: base.Add(2 * time.Second)},
			},
		},
		maintenanceResults: map[string]*coredata.ConversationMaintenanceResult{
			"eligible": {Eligible: true, Reason: coredata.ConversationMaintenanceEligible},
			"live":     {Reason: coredata.ConversationMaintenanceLiveRun},
		},
		maintenanceErrors: map[string]error{"broken": errors.New("database unavailable")},
	}
	policy := newInteractiveConversationCleanupPolicy(data, 30*24*time.Hour, false)
	policy.now = func() time.Time { return time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC) }
	worker := newTestConversationCleanupWorker(agentlyrt.ConversationCleanupOptions{
		Enabled:   true,
		BatchSize: 3,
		Timeout:   time.Second,
	}, policy)

	result, err := worker.runPass(context.Background())
	if err != nil {
		t.Fatalf("runPass() error: %v", err)
	}
	if result.Scanned != 3 || result.Eligible != 1 || result.Deleted != 0 || result.Skipped != 1 || result.Failed != 1 {
		t.Fatalf("result = %#v", result)
	}
	wantReasons := map[string]int{"eligible": 1, "live_run": 1, "error": 1}
	if !reflect.DeepEqual(result.Reasons, wantReasons) {
		t.Fatalf("reasons = %#v, want %#v", result.Reasons, wantReasons)
	}
	for _, request := range data.maintenanceRequests {
		if request.Mode != coredata.ConversationMaintenanceDryRun {
			t.Fatalf("non-dry-run maintenance request: %#v", request)
		}
	}
}

func TestInteractiveConversationCleanupPolicyUsesDeleteModeOnlyWhenEnabled(t *testing.T) {
	cutoff := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	data := &recordingConversationCleanupData{
		maintenanceResults: map[string]*coredata.ConversationMaintenanceResult{
			"root-delete": {
				RootID:   "root-delete",
				Kind:     coredata.ConversationMaintenanceInteractive,
				Mode:     coredata.ConversationMaintenanceDelete,
				Eligible: true,
				Deleted:  true,
				Reason:   coredata.ConversationMaintenanceDeleted,
			},
		},
	}
	policy := newInteractiveConversationCleanupPolicy(data, 30*24*time.Hour, true)
	policy.cutoff = cutoff
	lease := coredata.MaintenanceLease{Key: "conversation_cleanup", OwnerID: "worker", Token: "token"}
	ctx := context.WithValue(context.Background(), conversationCleanupLeaseContextKey{}, lease)

	outcome, err := policy.ProcessCandidate(ctx, conversationCleanupCandidate{
		RootID:          "root-delete",
		ExpectedOwnerID: "owner-1",
	})
	if err != nil {
		t.Fatalf("ProcessCandidate() error: %v", err)
	}
	if !outcome.Eligible || !outcome.Deleted || outcome.Reason != string(coredata.ConversationMaintenanceDeleted) {
		t.Fatalf("outcome = %#v", outcome)
	}
	if len(data.maintenanceRequests) != 1 {
		t.Fatalf("maintenance requests = %#v", data.maintenanceRequests)
	}
	request := data.maintenanceRequests[0]
	if request.Mode != coredata.ConversationMaintenanceDelete || request.RootID != "root-delete" ||
		request.ExpectedOwnerID != "owner-1" || request.Kind != coredata.ConversationMaintenanceInteractive ||
		!request.InactiveBefore.Equal(cutoff) || request.Lease != lease {
		t.Fatalf("maintenance request = %#v", request)
	}
}

type recordingConversationCleanupData struct {
	leaseMu                      sync.Mutex
	lease                        coredata.MaintenanceLease
	leaseSequence                int
	leaseRenewals                int
	leaseRenewError              error
	expiredLeasesDeleted         int64
	candidatePages               [][]coredata.ConversationMaintenanceCandidate
	candidateRequests            []coredata.ConversationMaintenanceCandidateRequest
	maintenanceResults           map[string]*coredata.ConversationMaintenanceResult
	maintenanceErrors            map[string]error
	maintenanceRequests          []coredata.ConversationMaintenanceRequest
	scheduledCandidatePages      [][]coredata.ScheduledRunMaintenanceCandidate
	scheduledCandidateRequests   []coredata.ScheduledRunMaintenanceCandidateRequest
	scheduledMaintenanceResults  map[string]*coredata.ScheduledRunMaintenanceResult
	scheduledMaintenanceErrors   map[string]error
	scheduledMaintenanceRequests []coredata.ScheduledRunMaintenanceRequest
	orphanCandidatePages         [][]coredata.OrphanMaintenanceCandidate
	orphanCandidateRequests      []coredata.OrphanMaintenanceCandidateRequest
	orphanMaintenanceResults     map[string]*coredata.OrphanMaintenanceResult
	orphanMaintenanceErrors      map[string]error
	orphanMaintenanceRequests    []coredata.OrphanMaintenanceRequest
	technicalCandidatePages      [][]coredata.TechnicalMaintenanceCandidate
	technicalCandidateRequests   []coredata.TechnicalMaintenanceCandidateRequest
	technicalMaintenanceResults  map[string]*coredata.TechnicalMaintenanceResult
	technicalMaintenanceErrors   map[string]error
	technicalMaintenanceRequests []coredata.TechnicalMaintenanceRequest
}

func (d *recordingConversationCleanupData) ListConversationMaintenanceCandidates(_ context.Context, request coredata.ConversationMaintenanceCandidateRequest) ([]coredata.ConversationMaintenanceCandidate, error) {
	d.candidateRequests = append(d.candidateRequests, request)
	index := len(d.candidateRequests) - 1
	if index >= len(d.candidatePages) {
		return nil, nil
	}
	return d.candidatePages[index], nil
}

func (d *recordingConversationCleanupData) MaintainConversationTree(_ context.Context, request coredata.ConversationMaintenanceRequest) (*coredata.ConversationMaintenanceResult, error) {
	d.maintenanceRequests = append(d.maintenanceRequests, request)
	if err := d.maintenanceErrors[request.RootID]; err != nil {
		return nil, err
	}
	return d.maintenanceResults[request.RootID], nil
}

func (d *recordingConversationCleanupData) ListScheduledRunMaintenanceCandidates(_ context.Context, request coredata.ScheduledRunMaintenanceCandidateRequest) ([]coredata.ScheduledRunMaintenanceCandidate, error) {
	d.scheduledCandidateRequests = append(d.scheduledCandidateRequests, request)
	index := len(d.scheduledCandidateRequests) - 1
	if index >= len(d.scheduledCandidatePages) {
		return nil, nil
	}
	return d.scheduledCandidatePages[index], nil
}

func (d *recordingConversationCleanupData) MaintainScheduledRun(_ context.Context, request coredata.ScheduledRunMaintenanceRequest) (*coredata.ScheduledRunMaintenanceResult, error) {
	d.scheduledMaintenanceRequests = append(d.scheduledMaintenanceRequests, request)
	if err := d.scheduledMaintenanceErrors[request.RunID]; err != nil {
		return nil, err
	}
	return d.scheduledMaintenanceResults[request.RunID], nil
}

func (d *recordingConversationCleanupData) ListOrphanMaintenanceCandidates(_ context.Context, request coredata.OrphanMaintenanceCandidateRequest) ([]coredata.OrphanMaintenanceCandidate, error) {
	d.orphanCandidateRequests = append(d.orphanCandidateRequests, request)
	index := len(d.orphanCandidateRequests) - 1
	if index >= len(d.orphanCandidatePages) {
		return nil, nil
	}
	return d.orphanCandidatePages[index], nil
}

func (d *recordingConversationCleanupData) MaintainOrphanCandidate(_ context.Context, request coredata.OrphanMaintenanceRequest) (*coredata.OrphanMaintenanceResult, error) {
	d.orphanMaintenanceRequests = append(d.orphanMaintenanceRequests, request)
	key := request.RuleID + "\x1e" + request.RecordID
	if err := d.orphanMaintenanceErrors[key]; err != nil {
		return nil, err
	}
	return d.orphanMaintenanceResults[key], nil
}

func (d *recordingConversationCleanupData) ListTechnicalMaintenanceCandidates(_ context.Context, request coredata.TechnicalMaintenanceCandidateRequest) ([]coredata.TechnicalMaintenanceCandidate, error) {
	d.technicalCandidateRequests = append(d.technicalCandidateRequests, request)
	index := len(d.technicalCandidateRequests) - 1
	if index >= len(d.technicalCandidatePages) {
		return nil, nil
	}
	return d.technicalCandidatePages[index], nil
}

func (d *recordingConversationCleanupData) MaintainTechnicalCandidate(_ context.Context, request coredata.TechnicalMaintenanceRequest) (*coredata.TechnicalMaintenanceResult, error) {
	d.technicalMaintenanceRequests = append(d.technicalMaintenanceRequests, request)
	key := string(request.Kind) + "\x1e" + request.RecordID
	if err := d.technicalMaintenanceErrors[key]; err != nil {
		return nil, err
	}
	return d.technicalMaintenanceResults[key], nil
}

func (d *recordingConversationCleanupData) AcquireMaintenanceLease(_ context.Context, request coredata.MaintenanceLeaseAcquireRequest) (*coredata.MaintenanceLeaseAcquireResult, error) {
	d.leaseMu.Lock()
	defer d.leaseMu.Unlock()
	now := time.Now().UTC()
	if d.lease.Token != "" && d.lease.LeaseUntil.After(now) {
		current := d.lease
		current.Token = ""
		return &coredata.MaintenanceLeaseAcquireResult{Lease: current}, nil
	}
	d.leaseSequence++
	d.lease = coredata.MaintenanceLease{
		Key: request.Key, OwnerID: request.OwnerID,
		Token:      request.OwnerID + "-token-" + strconv.Itoa(d.leaseSequence),
		LeaseUntil: now.Add(request.TTL),
	}
	return &coredata.MaintenanceLeaseAcquireResult{Acquired: true, Lease: d.lease}, nil
}

func (d *recordingConversationCleanupData) RenewMaintenanceLease(_ context.Context, lease coredata.MaintenanceLease, ttl time.Duration) (*coredata.MaintenanceLeaseRenewResult, error) {
	d.leaseMu.Lock()
	defer d.leaseMu.Unlock()
	if d.leaseRenewError != nil {
		return nil, d.leaseRenewError
	}
	now := time.Now().UTC()
	if d.lease.Key != lease.Key || d.lease.OwnerID != lease.OwnerID || d.lease.Token != lease.Token || !d.lease.LeaseUntil.After(now) {
		return &coredata.MaintenanceLeaseRenewResult{}, nil
	}
	d.leaseRenewals++
	d.lease.LeaseUntil = now.Add(ttl)
	return &coredata.MaintenanceLeaseRenewResult{Renewed: true, LeaseUntil: d.lease.LeaseUntil}, nil
}

func (d *recordingConversationCleanupData) ReleaseMaintenanceLease(_ context.Context, lease coredata.MaintenanceLease) (bool, error) {
	d.leaseMu.Lock()
	defer d.leaseMu.Unlock()
	if d.lease.Key != lease.Key || d.lease.OwnerID != lease.OwnerID || d.lease.Token != lease.Token {
		return false, nil
	}
	d.lease.LeaseUntil = time.Now().UTC()
	return true, nil
}

func (d *recordingConversationCleanupData) DeleteExpiredMaintenanceLeases(_ context.Context, lease coredata.MaintenanceLease) (int64, error) {
	d.leaseMu.Lock()
	defer d.leaseMu.Unlock()
	if d.lease.Key != lease.Key || d.lease.OwnerID != lease.OwnerID || d.lease.Token != lease.Token {
		return 0, coredata.ErrMaintenanceLeaseLost
	}
	return d.expiredLeasesDeleted, nil
}
