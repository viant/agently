package agently

import (
	"context"
	"fmt"
	"time"

	coredata "github.com/viant/agently-core/app/store/data"
)

const (
	interactiveConversationCleanupDryRunPolicyName = "interactive_retention_dry_run"
	interactiveConversationCleanupDeletePolicyName = "interactive_retention_delete"
)

type interactiveConversationCleanupPolicy struct {
	data      conversationCleanupData
	retention time.Duration
	mode      coredata.ConversationMaintenanceMode
	now       func() time.Time
	cutoff    time.Time
}

func newInteractiveConversationCleanupPolicy(data conversationCleanupData, retention time.Duration, deleteEnabled bool) *interactiveConversationCleanupPolicy {
	if retention <= 0 {
		retention = 30 * 24 * time.Hour
	}
	mode := coredata.ConversationMaintenanceDryRun
	if deleteEnabled {
		mode = coredata.ConversationMaintenanceDelete
	}
	return &interactiveConversationCleanupPolicy{
		data:      data,
		retention: retention,
		mode:      mode,
		now:       time.Now,
	}
}

func (p *interactiveConversationCleanupPolicy) Name() string {
	if p != nil && p.mode == coredata.ConversationMaintenanceDelete {
		return interactiveConversationCleanupDeletePolicyName
	}
	return interactiveConversationCleanupDryRunPolicyName
}

func (p *interactiveConversationCleanupPolicy) SelectCandidates(ctx context.Context, cursor conversationCleanupCursor, limit int) ([]conversationCleanupCandidate, error) {
	if p == nil || p.data == nil {
		return nil, fmt.Errorf("conversation cleanup data service is unavailable")
	}
	if cursor.RootID == "" {
		p.cutoff = p.now().UTC().Add(-p.retention)
	}
	candidates, err := p.data.ListConversationMaintenanceCandidates(ctx, coredata.ConversationMaintenanceCandidateRequest{
		Kind:           coredata.ConversationMaintenanceInteractive,
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

func (p *interactiveConversationCleanupPolicy) ProcessCandidate(ctx context.Context, candidate conversationCleanupCandidate) (conversationCleanupOutcome, error) {
	if p == nil || p.data == nil {
		return conversationCleanupOutcome{}, fmt.Errorf("conversation cleanup data service is unavailable")
	}
	result, err := p.data.MaintainConversationTree(ctx, coredata.ConversationMaintenanceRequest{
		RootID:          candidate.RootID,
		ExpectedOwnerID: candidate.ExpectedOwnerID,
		Kind:            coredata.ConversationMaintenanceInteractive,
		InactiveBefore:  p.cutoff,
		Mode:            p.mode,
		Lease:           conversationCleanupLease(ctx),
	})
	if err != nil {
		return conversationCleanupOutcome{}, err
	}
	if result == nil {
		return conversationCleanupOutcome{}, fmt.Errorf("conversation cleanup returned no result for %q", candidate.RootID)
	}
	return conversationCleanupOutcome{
		Eligible: result.Eligible,
		Deleted:  result.Deleted,
		Reason:   string(result.Reason),
	}, nil
}
