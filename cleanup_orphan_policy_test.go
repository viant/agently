package agently

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	coredata "github.com/viant/agently-core/app/store/data"
	agentlyrt "github.com/viant/agently/runtime"
)

func TestConversationCleanupPoliciesRegistersRequestedOrphanMode(t *testing.T) {
	data := &recordingConversationCleanupData{}
	policies := conversationCleanupPolicies(data, agentlyrt.ConversationCleanupOptions{
		OrphanMode: agentlyrt.ConversationCleanupModeDryRun, OrphanMinAge: 36 * time.Hour,
	})
	if len(policies) != 2 || policies[0].Name() != "technical_retention_unclassified_dry_run" || policies[1].Name() != orphanReportPolicyName {
		t.Fatalf("orphan policies = %#v", policies)
	}
	policy, ok := policies[1].(*orphanReportPolicy)
	if !ok || policy.gracePeriod != 36*time.Hour || policy.mutate {
		t.Fatalf("orphan policy = %#v", policies[1])
	}

	policies = conversationCleanupPolicies(data, agentlyrt.ConversationCleanupOptions{
		OrphanMode: agentlyrt.ConversationCleanupModeExecute, OrphanMinAge: 48 * time.Hour,
	})
	if len(policies) != 2 || policies[0].Name() != "technical_retention_unclassified_delete" || policies[1].Name() != orphanCleanupPolicyName {
		t.Fatalf("orphan cleanup policies = %#v", policies)
	}
	policy, ok = policies[1].(*orphanReportPolicy)
	if !ok || policy.gracePeriod != 48*time.Hour || !policy.mutate {
		t.Fatalf("orphan cleanup policy = %#v", policies[1])
	}
}

func TestOrphanReportPolicyUsesStableGraceCutoffAndKeyset(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	old := now.Add(-48 * time.Hour)
	data := &recordingConversationCleanupData{
		orphanCandidatePages: [][]coredata.OrphanMaintenanceCandidate{
			{{
				CursorID: "message.missing_turn\x1emessage-1", RuleID: "message.missing_turn",
				Action: coredata.OrphanMaintenanceSafeDetach, Table: "message", RecordID: "message-1",
				ReferenceTable: "turn", ReferenceID: "missing-turn", ObservedAt: old,
			}},
			nil,
			nil,
		},
	}
	policy := newOrphanReportPolicy(data, 24*time.Hour)
	policy.now = func() time.Time { return now }

	candidates, err := policy.SelectCandidates(context.Background(), conversationCleanupCursor{}, 7)
	if err != nil {
		t.Fatalf("SelectCandidates(first): %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidates = %#v", candidates)
	}
	candidate := candidates[0]
	if candidate.RuleID != "message.missing_turn" || candidate.RecordID != "message-1" || candidate.OrphanAction != coredata.OrphanMaintenanceSafeDetach || !candidate.ObservedAt.Equal(old) {
		t.Fatalf("candidate = %#v", candidate)
	}
	outcome, err := policy.ProcessCandidate(context.Background(), candidate)
	if err != nil {
		t.Fatalf("ProcessCandidate(): %v", err)
	}
	if !outcome.Eligible || outcome.Deleted || outcome.Reason != "orphan_safe-detach:message.missing_turn" {
		t.Fatalf("outcome = %#v", outcome)
	}

	now = now.Add(time.Hour)
	if _, err = policy.SelectCandidates(context.Background(), candidate.cursor(), 7); err != nil {
		t.Fatalf("SelectCandidates(next page): %v", err)
	}
	if _, err = policy.SelectCandidates(context.Background(), conversationCleanupCursor{}, 7); err != nil {
		t.Fatalf("SelectCandidates(next pass): %v", err)
	}
	if len(data.orphanCandidateRequests) != 3 {
		t.Fatalf("requests = %#v", data.orphanCandidateRequests)
	}
	firstCutoff := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	if !data.orphanCandidateRequests[0].OlderThan.Equal(firstCutoff) ||
		!data.orphanCandidateRequests[1].OlderThan.Equal(firstCutoff) ||
		!data.orphanCandidateRequests[2].OlderThan.Equal(firstCutoff.Add(time.Hour)) {
		t.Fatalf("cutoffs are not stable per pass: %#v", data.orphanCandidateRequests)
	}
	if data.orphanCandidateRequests[1].AfterCursor != candidate.RootID || data.orphanCandidateRequests[1].Limit != 7 {
		t.Fatalf("next request = %#v", data.orphanCandidateRequests[1])
	}
}

func TestOrphanReportWorkerOnlyReportsAndAggregatesRules(t *testing.T) {
	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	data := &recordingConversationCleanupData{
		orphanCandidatePages: [][]coredata.OrphanMaintenanceCandidate{{
			{CursorID: "call_payload.unused\x1epayload-1", RuleID: "call_payload.unused", Action: coredata.OrphanMaintenanceSafeDelete, Table: "call_payload", RecordID: "payload-1", ObservedAt: old},
			{CursorID: "investigation.missing_conversation\x1einvestigation-1", RuleID: "investigation.missing_conversation", Action: coredata.OrphanMaintenanceSafeDetach, Table: "investigation", RecordID: "investigation-1", ObservedAt: old},
			{CursorID: "report_audit_event.missing_job\x1eevent-1", RuleID: "report_audit_event.missing_job", Action: coredata.OrphanMaintenanceReportOnly, Table: "report_audit_event", RecordID: "event-1", ObservedAt: old},
		}},
	}
	policy := newOrphanReportPolicy(data, 24*time.Hour)
	policy.now = func() time.Time { return time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC) }
	worker := newTestConversationCleanupWorker(agentlyrt.ConversationCleanupOptions{Enabled: true, BatchSize: 3, Timeout: time.Second}, policy)

	result, err := worker.runPass(context.Background())
	if err != nil {
		t.Fatalf("runPass(): %v", err)
	}
	if result.Scanned != 3 || result.Eligible != 3 || result.Deleted != 0 || result.Mutated != 0 || result.Skipped != 0 || result.Failed != 0 {
		t.Fatalf("result = %#v", result)
	}
	if len(result.Reasons) != 3 {
		t.Fatalf("reasons = %#v", result.Reasons)
	}
	for reason := range result.Reasons {
		if !strings.HasPrefix(reason, "orphan_") {
			t.Fatalf("unexpected reason %q", reason)
		}
	}
}

func TestOrphanCleanupWorkerMutatesAllowedRulesAndContinuesAfterFailure(t *testing.T) {
	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cutoff := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	candidates := []coredata.OrphanMaintenanceCandidate{
		{CursorID: "000100\x1dmessage.missing_turn\x1emessage-1", RuleID: "message.missing_turn", Action: coredata.OrphanMaintenanceSafeDetach, Table: "message", RecordID: "message-1", ObservedAt: old},
		{CursorID: "000170\x1dschedule.missing_conversation\x1eschedule-1", RuleID: "schedule.missing_conversation", Action: coredata.OrphanMaintenanceSafeDetach, Table: "schedule", RecordID: "schedule-1", ObservedAt: old},
		{CursorID: "001210\x1dcall_payload.unused\x1epayload-1", RuleID: "call_payload.unused", Action: coredata.OrphanMaintenanceSafeDelete, Table: "call_payload", RecordID: "payload-1", ObservedAt: old},
		{CursorID: "002150\x1dreport_audit_event.missing_job\x1eevent-1", RuleID: "report_audit_event.missing_job", Action: coredata.OrphanMaintenanceReportOnly, Table: "report_audit_event", RecordID: "event-1", ObservedAt: old},
	}
	data := &recordingConversationCleanupData{
		orphanCandidatePages: [][]coredata.OrphanMaintenanceCandidate{candidates, nil},
		orphanMaintenanceResults: map[string]*coredata.OrphanMaintenanceResult{
			"message.missing_turn\x1emessage-1": {
				RuleID: "message.missing_turn", RecordID: "message-1", Action: coredata.OrphanMaintenanceSafeDetach,
				Eligible: true, Mutated: true, Detached: true, Reason: coredata.OrphanMaintenanceDetachedReason,
			},
			"call_payload.unused\x1epayload-1": {
				RuleID: "call_payload.unused", RecordID: "payload-1", Action: coredata.OrphanMaintenanceSafeDelete,
				Eligible: true, Mutated: true, Deleted: true, Reason: coredata.OrphanMaintenanceDeletedReason,
			},
			"report_audit_event.missing_job\x1eevent-1": {
				RuleID: "report_audit_event.missing_job", RecordID: "event-1", Action: coredata.OrphanMaintenanceReportOnly,
				Eligible: true, Reason: coredata.OrphanMaintenanceReportOnlyReason,
			},
		},
		orphanMaintenanceErrors: map[string]error{
			"schedule.missing_conversation\x1eschedule-1": errors.New("simulated rule failure"),
		},
	}
	policy := newOrphanMaintenancePolicy(data, 24*time.Hour, true)
	policy.now = func() time.Time { return cutoff.Add(24 * time.Hour) }
	worker := newTestConversationCleanupWorker(agentlyrt.ConversationCleanupOptions{
		Enabled: true, Debug: true, BatchSize: len(candidates), Timeout: time.Second,
	}, policy)

	result, err := worker.runPass(context.Background())
	if err != nil {
		t.Fatalf("runPass(): %v", err)
	}
	if result.Scanned != 4 || result.Eligible != 3 || result.Deleted != 1 || result.Mutated != 2 || result.Skipped != 0 || result.Failed != 1 {
		t.Fatalf("result = %#v", result)
	}
	wantReasons := map[string]int{
		"error": 1,
		"orphan_safe-delete:call_payload.unused:deleted":                1,
		"orphan_safe-detach:message.missing_turn:detached":              1,
		"orphan_report-only:report_audit_event.missing_job:report_only": 1,
	}
	if !reflect.DeepEqual(result.Reasons, wantReasons) {
		t.Fatalf("reasons = %#v, want %#v", result.Reasons, wantReasons)
	}
	if len(data.orphanMaintenanceRequests) != 4 {
		t.Fatalf("maintenance requests = %#v", data.orphanMaintenanceRequests)
	}
	for _, request := range data.orphanMaintenanceRequests {
		if !request.OlderThan.Equal(cutoff) || request.Lease.Token == "" {
			t.Fatalf("maintenance request = %#v, cutoff=%s", request, cutoff)
		}
	}
}

func TestOrphanCleanupPolicySkipsCandidateThatRecoveredBeforeRecheck(t *testing.T) {
	cutoff := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	data := &recordingConversationCleanupData{
		orphanMaintenanceResults: map[string]*coredata.OrphanMaintenanceResult{
			"message.missing_linked_conversation\x1emessage-1": {
				RuleID: "message.missing_linked_conversation", RecordID: "message-1",
				Action: coredata.OrphanMaintenanceSafeDetach, Reason: coredata.OrphanMaintenanceNoLongerEligibleReason,
			},
		},
	}
	policy := newOrphanMaintenancePolicy(data, 24*time.Hour, true)
	policy.cutoff = cutoff
	lease := coredata.MaintenanceLease{Key: "conversation_cleanup", OwnerID: "worker", Token: "token"}
	ctx := context.WithValue(context.Background(), conversationCleanupLeaseContextKey{}, lease)
	outcome, err := policy.ProcessCandidate(ctx, conversationCleanupCandidate{
		RuleID: "message.missing_linked_conversation", RecordID: "message-1",
		OrphanAction: coredata.OrphanMaintenanceSafeDetach,
	})
	if err != nil {
		t.Fatalf("ProcessCandidate(): %v", err)
	}
	if outcome.Eligible || outcome.Deleted || outcome.Mutated || outcome.Reason != "orphan_safe-detach:message.missing_linked_conversation:no_longer_eligible" {
		t.Fatalf("outcome = %#v", outcome)
	}
	if len(data.orphanMaintenanceRequests) != 1 || data.orphanMaintenanceRequests[0].Lease != lease {
		t.Fatalf("maintenance requests = %#v", data.orphanMaintenanceRequests)
	}
}
