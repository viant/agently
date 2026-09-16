package agently

import (
	"context"
	"fmt"
	"time"

	coredata "github.com/viant/agently-core/app/store/data"
)

const (
	scheduledConversationFallbackDryRunPolicyName = "scheduled_conversation_fallback_dry_run"
	scheduledConversationFallbackDeletePolicyName = "scheduled_conversation_fallback_delete"
)

// scheduledConversationFallbackPolicy removes old scheduler-marked
// conversation graphs which are not represented by either the current run
// table or the legacy schedule_run table. It shares configuration and
// retention with scheduled-run cleanup, while core rechecks the no-run
// condition transactionally before deleting anything.
type scheduledConversationFallbackPolicy struct {
	data      conversationCleanupData
	retention time.Duration
	mode      coredata.ConversationMaintenanceMode
	now       func() time.Time
	cutoff    time.Time
}

func newScheduledConversationFallbackPolicy(data conversationCleanupData, retention time.Duration, deleteEnabled bool) *scheduledConversationFallbackPolicy {
	if retention <= 0 {
		retention = 30 * 24 * time.Hour
	}
	mode := coredata.ConversationMaintenanceDryRun
	if deleteEnabled {
		mode = coredata.ConversationMaintenanceDelete
	}
	return &scheduledConversationFallbackPolicy{
		data:      data,
		retention: retention,
		mode:      mode,
		now:       time.Now,
	}
}

func (p *scheduledConversationFallbackPolicy) Name() string {
	if p != nil && p.mode == coredata.ConversationMaintenanceDelete {
		return scheduledConversationFallbackDeletePolicyName
	}
	return scheduledConversationFallbackDryRunPolicyName
}

func (p *scheduledConversationFallbackPolicy) SelectCandidates(ctx context.Context, cursor conversationCleanupCursor, limit int) ([]conversationCleanupCandidate, error) {
	if p == nil || p.data == nil {
		return nil, fmt.Errorf("conversation cleanup data service is unavailable")
	}
	if cursor.RootID == "" {
		p.cutoff = p.now().UTC().Add(-p.retention)
	}
	candidates, err := p.data.ListConversationMaintenanceCandidates(ctx, coredata.ConversationMaintenanceCandidateRequest{
		Kind:           coredata.ConversationMaintenanceScheduledFallback,
		InactiveBefore: p.cutoff,
		AfterActivity:  cursor.ActivityAt,
		AfterRootID:    cursor.RootID,
		Limit:          limit,
	})
	if err != nil {
		return nil, err
	}
	result := make([]conversationCleanupCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		result = append(result, conversationCleanupCandidate{
			RootID:          candidate.RootID,
			ExpectedOwnerID: candidate.ExpectedOwnerID,
			ActivityAt:      candidate.ActivityAt,
		})
	}
	return result, nil
}

func (p *scheduledConversationFallbackPolicy) ProcessCandidate(ctx context.Context, candidate conversationCleanupCandidate) (conversationCleanupOutcome, error) {
	if p == nil || p.data == nil {
		return conversationCleanupOutcome{}, fmt.Errorf("conversation cleanup data service is unavailable")
	}
	result, err := p.data.MaintainConversationTree(ctx, coredata.ConversationMaintenanceRequest{
		RootID:          candidate.RootID,
		ExpectedOwnerID: candidate.ExpectedOwnerID,
		Kind:            coredata.ConversationMaintenanceScheduledFallback,
		InactiveBefore:  p.cutoff,
		Mode:            p.mode,
		Lease:           conversationCleanupLease(ctx),
	})
	if err != nil {
		return conversationCleanupOutcome{}, err
	}
	if result == nil {
		return conversationCleanupOutcome{}, fmt.Errorf("scheduled conversation fallback returned no result for %q", candidate.RootID)
	}
	return conversationCleanupOutcome{
		Eligible: result.Eligible,
		Deleted:  result.Deleted,
		Reason:   string(result.Reason),
	}, nil
}
