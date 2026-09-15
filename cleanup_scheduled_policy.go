package agently

import (
	"context"
	"fmt"
	"time"

	coredata "github.com/viant/agently-core/app/store/data"
)

const (
	scheduledRunCleanupDryRunPolicyName = "scheduled_retention_dry_run"
	scheduledRunCleanupDeletePolicyName = "scheduled_retention_delete"
)

type scheduledRunCleanupPolicy struct {
	data      conversationCleanupData
	retention time.Duration
	deleteRun bool
	now       func() time.Time
	cutoff    time.Time
}

func newScheduledRunCleanupPolicy(data conversationCleanupData, retention time.Duration, deleteRun bool) *scheduledRunCleanupPolicy {
	if retention <= 0 {
		retention = 30 * 24 * time.Hour
	}
	return &scheduledRunCleanupPolicy{
		data:      data,
		retention: retention,
		deleteRun: deleteRun,
		now:       time.Now,
	}
}

func (p *scheduledRunCleanupPolicy) Name() string {
	if p != nil && p.deleteRun {
		return scheduledRunCleanupDeletePolicyName
	}
	return scheduledRunCleanupDryRunPolicyName
}

func (p *scheduledRunCleanupPolicy) SelectCandidates(ctx context.Context, cursor conversationCleanupCursor, limit int) ([]conversationCleanupCandidate, error) {
	if p == nil || p.data == nil {
		return nil, fmt.Errorf("conversation cleanup data service is unavailable")
	}
	if cursor.RootID == "" {
		p.cutoff = p.now().UTC().Add(-p.retention)
	}
	candidates, err := p.data.ListScheduledRunMaintenanceCandidates(ctx, coredata.ScheduledRunMaintenanceCandidateRequest{
		InactiveBefore: p.cutoff,
		AfterActivity:  cursor.ActivityAt,
		AfterRunID:     cursor.RootID,
		Limit:          limit,
	})
	if err != nil {
		return nil, err
	}
	result := make([]conversationCleanupCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		result = append(result, conversationCleanupCandidate{
			RootID:          candidate.RunID,
			ExpectedOwnerID: candidate.ExpectedOwnerID,
			ActivityAt:      candidate.ActivityAt,
		})
	}
	return result, nil
}

func (p *scheduledRunCleanupPolicy) ProcessCandidate(ctx context.Context, candidate conversationCleanupCandidate) (conversationCleanupOutcome, error) {
	if p == nil || p.data == nil {
		return conversationCleanupOutcome{}, fmt.Errorf("conversation cleanup data service is unavailable")
	}
	result, err := p.data.MaintainScheduledRun(ctx, coredata.ScheduledRunMaintenanceRequest{
		RunID:           candidate.RootID,
		ExpectedOwnerID: candidate.ExpectedOwnerID,
		InactiveBefore:  p.cutoff,
		Mode:            p.maintenanceMode(),
		Lease:           conversationCleanupLease(ctx),
	})
	if err != nil {
		return conversationCleanupOutcome{}, err
	}
	if result == nil {
		return conversationCleanupOutcome{}, fmt.Errorf("scheduled cleanup returned no result for %q", candidate.RootID)
	}
	return conversationCleanupOutcome{
		Eligible: result.Eligible,
		Deleted:  result.Deleted,
		Reason:   string(result.Reason),
	}, nil
}

func (p *scheduledRunCleanupPolicy) maintenanceMode() coredata.ConversationMaintenanceMode {
	if p != nil && p.deleteRun {
		return coredata.ConversationMaintenanceDelete
	}
	return coredata.ConversationMaintenanceDryRun
}
