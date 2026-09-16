package agently

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	coredata "github.com/viant/agently-core/app/store/data"
	agentlyrt "github.com/viant/agently/runtime"
)

func TestConversationCleanupPoliciesRegistersScheduledRetentionInSelectedMode(t *testing.T) {
	data := &recordingConversationCleanupData{}

	policies := conversationCleanupPolicies(data, agentlyrt.ConversationCleanupOptions{
		ScheduledMode: agentlyrt.ConversationCleanupModeExecute,
	})
	if len(policies) != 3 || policies[0].Name() != scheduledRunCleanupDeletePolicyName || policies[1].Name() != scheduledConversationFallbackDeletePolicyName || policies[2].Name() != "technical_retention_scheduled_delete" {
		t.Fatalf("scheduled delete policies = %#v", policies)
	}
	deletePolicy, ok := policies[0].(*scheduledRunCleanupPolicy)
	if !ok || !deletePolicy.deleteRun {
		t.Fatalf("scheduled delete policy = %#v", policies[0])
	}

	policies = conversationCleanupPolicies(data, agentlyrt.ConversationCleanupOptions{
		ScheduledMode:      agentlyrt.ConversationCleanupModeDryRun,
		ScheduledRetention: 45 * 24 * time.Hour,
	})
	if len(policies) != 3 || policies[0].Name() != scheduledRunCleanupDryRunPolicyName || policies[1].Name() != scheduledConversationFallbackDryRunPolicyName || policies[2].Name() != "technical_retention_scheduled_dry_run" {
		t.Fatalf("scheduled policies = %#v", policies)
	}
	policy, ok := policies[0].(*scheduledRunCleanupPolicy)
	if !ok || policy.retention != 45*24*time.Hour || policy.deleteRun {
		t.Fatalf("scheduled policy = %#v", policies[0])
	}

	policies = conversationCleanupPolicies(data, agentlyrt.ConversationCleanupOptions{
		InteractiveMode:      agentlyrt.ConversationCleanupModeDryRun,
		InteractiveRetention: 30 * 24 * time.Hour,
		ScheduledMode:        agentlyrt.ConversationCleanupModeDryRun,
		ScheduledRetention:   60 * 24 * time.Hour,
		OrphanMode:           agentlyrt.ConversationCleanupModeDryRun,
		OrphanMinAge:         24 * time.Hour,
	})
	if len(policies) != 7 ||
		policies[0].Name() != interactiveConversationCleanupDryRunPolicyName ||
		policies[1].Name() != "technical_retention_interactive_dry_run" ||
		policies[2].Name() != scheduledRunCleanupDryRunPolicyName ||
		policies[3].Name() != scheduledConversationFallbackDryRunPolicyName ||
		policies[4].Name() != "technical_retention_scheduled_dry_run" ||
		policies[5].Name() != "technical_retention_unclassified_dry_run" ||
		policies[6].Name() != orphanReportPolicyName {
		t.Fatalf("combined policies = %#v", policies)
	}
}

func TestScheduledConversationFallbackPolicyUsesScheduledRetentionAndFencedMode(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	activityAt := now.Add(-60 * 24 * time.Hour)
	data := &recordingConversationCleanupData{
		candidatePages: [][]coredata.ConversationMaintenanceCandidate{{
			{RootID: "scheduled-shell", ActivityAt: activityAt},
		}},
		maintenanceResults: map[string]*coredata.ConversationMaintenanceResult{
			"scheduled-shell": {
				RootID:   "scheduled-shell",
				Kind:     coredata.ConversationMaintenanceScheduledFallback,
				Mode:     coredata.ConversationMaintenanceDelete,
				Eligible: true,
				Deleted:  true,
				Reason:   coredata.ConversationMaintenanceDeleted,
			},
		},
	}
	policy := newScheduledConversationFallbackPolicy(data, 45*24*time.Hour, true)
	policy.now = func() time.Time { return now }

	candidates, err := policy.SelectCandidates(context.Background(), conversationCleanupCursor{}, 7)
	if err != nil {
		t.Fatalf("SelectCandidates() error: %v", err)
	}
	if len(candidates) != 1 || candidates[0].RootID != "scheduled-shell" || !candidates[0].ActivityAt.Equal(activityAt) {
		t.Fatalf("candidates = %#v", candidates)
	}
	if len(data.candidateRequests) != 1 {
		t.Fatalf("candidate requests = %#v", data.candidateRequests)
	}
	wantCutoff := now.Add(-45 * 24 * time.Hour)
	candidateRequest := data.candidateRequests[0]
	if candidateRequest.Kind != coredata.ConversationMaintenanceScheduledFallback ||
		!candidateRequest.InactiveBefore.Equal(wantCutoff) || candidateRequest.Limit != 7 {
		t.Fatalf("candidate request = %#v", candidateRequest)
	}

	lease := coredata.MaintenanceLease{Key: "conversation_cleanup", OwnerID: "worker", Token: "token"}
	ctx := context.WithValue(context.Background(), conversationCleanupLeaseContextKey{}, lease)
	outcome, err := policy.ProcessCandidate(ctx, candidates[0])
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
	if request.RootID != "scheduled-shell" || request.ExpectedOwnerID != "" ||
		request.Kind != coredata.ConversationMaintenanceScheduledFallback ||
		request.Mode != coredata.ConversationMaintenanceDelete ||
		!request.InactiveBefore.Equal(wantCutoff) || request.Lease != lease {
		t.Fatalf("maintenance request = %#v", request)
	}
}

func TestScheduledRunCleanupPolicyUsesStableCutoffAndKeyset(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	activityAt := now.Add(-60 * 24 * time.Hour)
	data := &recordingConversationCleanupData{
		scheduledCandidatePages: [][]coredata.ScheduledRunMaintenanceCandidate{
			{{RunID: "run-1", ExpectedOwnerID: "owner-1", ActivityAt: activityAt}},
			nil,
			nil,
		},
		scheduledMaintenanceResults: map[string]*coredata.ScheduledRunMaintenanceResult{
			"run-1": {
				RunID:    "run-1",
				Eligible: true,
				Reason:   coredata.ConversationMaintenanceEligible,
			},
		},
	}
	policy := newScheduledRunCleanupPolicy(data, 30*24*time.Hour, false)
	policy.now = func() time.Time { return now }

	candidates, err := policy.SelectCandidates(context.Background(), conversationCleanupCursor{}, 7)
	if err != nil {
		t.Fatalf("SelectCandidates(first) error: %v", err)
	}
	if len(candidates) != 1 || candidates[0].RootID != "run-1" || candidates[0].ExpectedOwnerID != "owner-1" || !candidates[0].ActivityAt.Equal(activityAt) {
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
	if _, err = policy.SelectCandidates(context.Background(), candidates[0].cursor(), 7); err != nil {
		t.Fatalf("SelectCandidates(next page) error: %v", err)
	}
	if _, err = policy.SelectCandidates(context.Background(), conversationCleanupCursor{}, 7); err != nil {
		t.Fatalf("SelectCandidates(next pass) error: %v", err)
	}

	firstCutoff := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	if len(data.scheduledCandidateRequests) != 3 {
		t.Fatalf("candidate requests = %#v", data.scheduledCandidateRequests)
	}
	if !data.scheduledCandidateRequests[0].InactiveBefore.Equal(firstCutoff) ||
		!data.scheduledCandidateRequests[1].InactiveBefore.Equal(firstCutoff) ||
		!data.scheduledCandidateRequests[2].InactiveBefore.Equal(firstCutoff.Add(24*time.Hour)) {
		t.Fatalf("candidate cutoffs are not stable per pass: %#v", data.scheduledCandidateRequests)
	}
	firstRequest := data.scheduledCandidateRequests[0]
	if firstRequest.Limit != 7 || !firstRequest.AfterActivity.IsZero() || firstRequest.AfterRunID != "" {
		t.Fatalf("first candidate request = %#v", firstRequest)
	}
	secondRequest := data.scheduledCandidateRequests[1]
	if !secondRequest.AfterActivity.Equal(activityAt) || secondRequest.AfterRunID != "run-1" {
		t.Fatalf("second candidate request = %#v", secondRequest)
	}
	if len(data.scheduledMaintenanceRequests) != 1 {
		t.Fatalf("maintenance requests = %#v", data.scheduledMaintenanceRequests)
	}
	request := data.scheduledMaintenanceRequests[0]
	if request.RunID != "run-1" || request.ExpectedOwnerID != "owner-1" || !request.InactiveBefore.Equal(firstCutoff) {
		t.Fatalf("maintenance request = %#v", request)
	}
}

func TestScheduledRunCleanupWorkerDryRunAggregatesReasonsWithoutDeletes(t *testing.T) {
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	data := &recordingConversationCleanupData{
		scheduledCandidatePages: [][]coredata.ScheduledRunMaintenanceCandidate{{
			{RunID: "ended", ExpectedOwnerID: "owner", ActivityAt: base},
			{RunID: "failed", ExpectedOwnerID: "owner", ActivityAt: base.Add(time.Second)},
			{RunID: "live", ExpectedOwnerID: "owner", ActivityAt: base.Add(2 * time.Second)},
			{RunID: "broken", ExpectedOwnerID: "owner", ActivityAt: base.Add(3 * time.Second)},
		}},
		scheduledMaintenanceResults: map[string]*coredata.ScheduledRunMaintenanceResult{
			"ended":  {Eligible: true, Reason: coredata.ConversationMaintenanceEligible},
			"failed": {Eligible: true, Reason: coredata.ConversationMaintenanceEligible},
			"live":   {Reason: coredata.ConversationMaintenanceLiveRun},
		},
		scheduledMaintenanceErrors: map[string]error{"broken": errors.New("database unavailable")},
	}
	policy := newScheduledRunCleanupPolicy(data, 30*24*time.Hour, false)
	policy.now = func() time.Time { return time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC) }
	worker := newTestConversationCleanupWorker(agentlyrt.ConversationCleanupOptions{
		Enabled:   true,
		BatchSize: 4,
		Timeout:   time.Second,
	}, policy)

	result, err := worker.runPass(context.Background())
	if err != nil {
		t.Fatalf("runPass() error: %v", err)
	}
	if result.Scanned != 4 || result.Eligible != 2 || result.Deleted != 0 || result.Skipped != 1 || result.Failed != 1 {
		t.Fatalf("result = %#v", result)
	}
	wantReasons := map[string]int{"eligible": 2, "live_run": 1, "error": 1}
	if !reflect.DeepEqual(result.Reasons, wantReasons) {
		t.Fatalf("reasons = %#v, want %#v", result.Reasons, wantReasons)
	}
	if len(data.scheduledMaintenanceRequests) != 4 {
		t.Fatalf("maintenance requests = %#v", data.scheduledMaintenanceRequests)
	}
}

func TestScheduledRunCleanupPolicyDeleteModeUsesFencedMaintenanceDelete(t *testing.T) {
	cutoff := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	data := &recordingConversationCleanupData{
		scheduledMaintenanceResults: map[string]*coredata.ScheduledRunMaintenanceResult{
			"eligible": {RunID: "eligible", Eligible: true, Deleted: true, Reason: coredata.ConversationMaintenanceDeleted},
			"live":     {RunID: "live", Reason: coredata.ConversationMaintenanceLiveRun},
		},
	}
	policy := newScheduledRunCleanupPolicy(data, 30*24*time.Hour, true)
	policy.cutoff = cutoff
	lease := coredata.MaintenanceLease{Key: "conversation_cleanup", OwnerID: "worker", Token: "token"}
	ctx := context.WithValue(context.Background(), conversationCleanupLeaseContextKey{}, lease)

	outcome, err := policy.ProcessCandidate(ctx, conversationCleanupCandidate{
		RootID:          "eligible",
		ExpectedOwnerID: "owner-1",
	})
	if err != nil {
		t.Fatalf("ProcessCandidate(eligible) error: %v", err)
	}
	if !outcome.Eligible || !outcome.Deleted || outcome.Reason != string(coredata.ConversationMaintenanceDeleted) {
		t.Fatalf("eligible outcome = %#v", outcome)
	}
	if len(data.scheduledMaintenanceRequests) != 1 || data.scheduledMaintenanceRequests[0].Mode != coredata.ConversationMaintenanceDelete || data.scheduledMaintenanceRequests[0].Lease != lease {
		t.Fatalf("maintenance calls = %#v", data.scheduledMaintenanceRequests)
	}

	outcome, err = policy.ProcessCandidate(ctx, conversationCleanupCandidate{
		RootID:          "live",
		ExpectedOwnerID: "owner-1",
	})
	if err != nil {
		t.Fatalf("ProcessCandidate(live) error: %v", err)
	}
	if outcome.Eligible || outcome.Deleted || outcome.Reason != string(coredata.ConversationMaintenanceLiveRun) {
		t.Fatalf("live outcome = %#v", outcome)
	}
	if len(data.scheduledMaintenanceRequests) != 2 {
		t.Fatalf("maintenance calls = %#v", data.scheduledMaintenanceRequests)
	}
}

func TestScheduledRunCleanupPolicyDeleteRetriesFailureAndTreatsMissingAsIdempotent(t *testing.T) {
	data := &recordingConversationCleanupData{
		scheduledMaintenanceResults: map[string]*coredata.ScheduledRunMaintenanceResult{
			"run-1": {RunID: "run-1", Eligible: true, Deleted: true, Reason: coredata.ConversationMaintenanceDeleted},
		},
		scheduledMaintenanceErrors: map[string]error{"run-1": errors.New("temporary delete failure")},
	}
	policy := newScheduledRunCleanupPolicy(data, 30*24*time.Hour, true)
	policy.cutoff = time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	candidate := conversationCleanupCandidate{RootID: "run-1", ExpectedOwnerID: "owner-1"}

	if _, err := policy.ProcessCandidate(context.Background(), candidate); err == nil {
		t.Fatal("first ProcessCandidate() error = nil")
	}
	delete(data.scheduledMaintenanceErrors, "run-1")
	outcome, err := policy.ProcessCandidate(context.Background(), candidate)
	if err != nil || !outcome.Deleted {
		t.Fatalf("retry outcome=%#v err=%v", outcome, err)
	}
	data.scheduledMaintenanceResults["run-1"] = &coredata.ScheduledRunMaintenanceResult{
		RunID: "run-1", Reason: coredata.ConversationMaintenanceNotFound,
	}
	outcome, err = policy.ProcessCandidate(context.Background(), candidate)
	if err != nil {
		t.Fatalf("idempotent missing run error: %v", err)
	}
	if outcome.Eligible || outcome.Deleted || outcome.Reason != string(coredata.ConversationMaintenanceNotFound) {
		t.Fatalf("idempotent missing outcome = %#v", outcome)
	}
	if len(data.scheduledMaintenanceRequests) != 3 {
		t.Fatalf("maintenance calls = %#v", data.scheduledMaintenanceRequests)
	}
}

func TestScheduledRunCleanupWorkerDeleteContinuesAfterCandidateFailure(t *testing.T) {
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	data := &recordingConversationCleanupData{
		scheduledCandidatePages: [][]coredata.ScheduledRunMaintenanceCandidate{{
			{RunID: "deleted-1", ExpectedOwnerID: "owner", ActivityAt: base},
			{RunID: "broken", ExpectedOwnerID: "owner", ActivityAt: base.Add(time.Second)},
			{RunID: "live", ExpectedOwnerID: "owner", ActivityAt: base.Add(2 * time.Second)},
			{RunID: "deleted-2", ExpectedOwnerID: "owner", ActivityAt: base.Add(3 * time.Second)},
		}},
		scheduledMaintenanceResults: map[string]*coredata.ScheduledRunMaintenanceResult{
			"deleted-1": {Eligible: true, Deleted: true, Reason: coredata.ConversationMaintenanceDeleted},
			"broken":    {Eligible: true, Deleted: true, Reason: coredata.ConversationMaintenanceDeleted},
			"live":      {Reason: coredata.ConversationMaintenanceLiveRun},
			"deleted-2": {Eligible: true, Deleted: true, Reason: coredata.ConversationMaintenanceDeleted},
		},
		scheduledMaintenanceErrors: map[string]error{"broken": errors.New("database unavailable")},
	}
	policy := newScheduledRunCleanupPolicy(data, 30*24*time.Hour, true)
	policy.now = func() time.Time { return time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC) }
	worker := newTestConversationCleanupWorker(agentlyrt.ConversationCleanupOptions{
		Enabled:   true,
		BatchSize: 4,
		Timeout:   time.Second,
	}, policy)

	result, err := worker.runPass(context.Background())
	if err != nil {
		t.Fatalf("runPass() error: %v", err)
	}
	if result.Scanned != 4 || result.Eligible != 2 || result.Deleted != 2 || result.Skipped != 1 || result.Failed != 1 {
		t.Fatalf("result = %#v", result)
	}
	wantReasons := map[string]int{"deleted": 2, "live_run": 1, "error": 1}
	if !reflect.DeepEqual(result.Reasons, wantReasons) {
		t.Fatalf("reasons = %#v, want %#v", result.Reasons, wantReasons)
	}
	if len(data.scheduledMaintenanceRequests) != 4 {
		t.Fatalf("maintenance calls = %#v", data.scheduledMaintenanceRequests)
	}
}
