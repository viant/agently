package agently

import (
	"context"
	"testing"
	"time"

	coredata "github.com/viant/agently-core/app/store/data"
	agentlyrt "github.com/viant/agently/runtime"
)

func TestTechnicalCleanupPolicyUsesStableCutoffCursorAndSelectedMode(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	observed := now.Add(-60 * 24 * time.Hour)
	cursorID := "0010\x1creport_run\x1crun-1"
	key := string(coredata.TechnicalMaintenanceReportRun) + "\x1e" + "run-1"
	data := &recordingConversationCleanupData{
		technicalCandidatePages: [][]coredata.TechnicalMaintenanceCandidate{
			{{CursorID: cursorID, Kind: coredata.TechnicalMaintenanceReportRun, Scope: coredata.TechnicalMaintenanceInteractive, RecordID: "run-1", ObservedAt: observed}},
			nil,
		},
		technicalMaintenanceResults: map[string]*coredata.TechnicalMaintenanceResult{
			key: {
				Kind: coredata.TechnicalMaintenanceReportRun, Scope: coredata.TechnicalMaintenanceInteractive,
				RecordID: "run-1", Mode: coredata.ConversationMaintenanceDelete,
				Eligible: true, Deleted: true, Reason: coredata.TechnicalMaintenanceDeletedReason,
			},
		},
	}
	policy := newTechnicalCleanupPolicy(data, coredata.TechnicalMaintenanceInteractive, 30*24*time.Hour, true)
	policy.now = func() time.Time { return now }

	candidates, err := policy.SelectCandidates(context.Background(), conversationCleanupCursor{}, 5)
	if err != nil || len(candidates) != 1 {
		t.Fatalf("SelectCandidates() candidates=%#v err=%v", candidates, err)
	}
	candidate := candidates[0]
	if candidate.RootID != cursorID || candidate.RecordID != "run-1" || candidate.TechnicalKind != coredata.TechnicalMaintenanceReportRun || !candidate.ActivityAt.Equal(now) {
		t.Fatalf("candidate = %#v", candidate)
	}
	_, err = policy.SelectCandidates(context.Background(), candidate.cursor(), 5)
	if err != nil {
		t.Fatalf("SelectCandidates(cursor) error: %v", err)
	}
	if len(data.technicalCandidateRequests) != 2 ||
		!data.technicalCandidateRequests[0].OlderThan.Equal(now.Add(-30*24*time.Hour)) ||
		!data.technicalCandidateRequests[0].EvaluatedAt.Equal(now) ||
		data.technicalCandidateRequests[1].AfterCursor != cursorID {
		t.Fatalf("candidate requests = %#v", data.technicalCandidateRequests)
	}

	lease := coredata.MaintenanceLease{Key: "conversation_cleanup", OwnerID: "worker", Token: "token"}
	ctx := context.WithValue(context.Background(), conversationCleanupLeaseContextKey{}, lease)
	outcome, err := policy.ProcessCandidate(ctx, candidate)
	if err != nil {
		t.Fatalf("ProcessCandidate() error: %v", err)
	}
	if !outcome.Eligible || !outcome.Deleted || outcome.Reason != "technical_report_run:deleted" {
		t.Fatalf("outcome = %#v", outcome)
	}
	if len(data.technicalMaintenanceRequests) != 1 {
		t.Fatalf("maintenance requests = %#v", data.technicalMaintenanceRequests)
	}
	request := data.technicalMaintenanceRequests[0]
	if request.Mode != coredata.ConversationMaintenanceDelete || request.Lease != lease ||
		!request.OlderThan.Equal(now.Add(-30*24*time.Hour)) || !request.EvaluatedAt.Equal(now) {
		t.Fatalf("maintenance request = %#v", request)
	}
}

func TestConversationCleanupPoliciesUsesLongerRetentionForUnclassifiedTechnicalState(t *testing.T) {
	policies := conversationCleanupPolicies(&recordingConversationCleanupData{}, agentlyrt.ConversationCleanupOptions{
		InteractiveRetention: 29 * 24 * time.Hour,
		ScheduledRetention:   45 * 24 * time.Hour,
		OrphanMode:           agentlyrt.ConversationCleanupModeDryRun,
		OrphanMinAge:         14 * 24 * time.Hour,
	})
	if len(policies) != 2 {
		t.Fatalf("policies = %#v", policies)
	}
	technical, ok := policies[0].(*technicalCleanupPolicy)
	if !ok || technical.scope != coredata.TechnicalMaintenanceUnclassified || technical.retention != 45*24*time.Hour || technical.mode != coredata.ConversationMaintenanceDryRun {
		t.Fatalf("unclassified technical policy = %#v", policies[0])
	}
}
