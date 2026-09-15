package agently

import (
	"context"
	"fmt"
	"time"

	coredata "github.com/viant/agently-core/app/store/data"
)

const (
	orphanReportPolicyName  = "orphan-report"
	orphanCleanupPolicyName = "orphan-cleanup"
)

type orphanReportPolicy struct {
	data        conversationCleanupData
	gracePeriod time.Duration
	cutoff      time.Time
	now         func() time.Time
	mutate      bool
}

func newOrphanReportPolicy(data conversationCleanupData, gracePeriod time.Duration) *orphanReportPolicy {
	return newOrphanMaintenancePolicy(data, gracePeriod, false)
}

func newOrphanMaintenancePolicy(data conversationCleanupData, gracePeriod time.Duration, mutate bool) *orphanReportPolicy {
	if gracePeriod <= 0 {
		gracePeriod = 24 * time.Hour
	}
	return &orphanReportPolicy{
		data: data, gracePeriod: gracePeriod, mutate: mutate,
		now: func() time.Time { return time.Now().UTC() },
	}
}

func (p *orphanReportPolicy) Name() string {
	if p.mutate {
		return orphanCleanupPolicyName
	}
	return orphanReportPolicyName
}

func (p *orphanReportPolicy) SelectCandidates(ctx context.Context, cursor conversationCleanupCursor, limit int) ([]conversationCleanupCandidate, error) {
	if cursor.RootID == "" {
		p.cutoff = p.now().UTC().Add(-p.gracePeriod)
	}
	rows, err := p.data.ListOrphanMaintenanceCandidates(ctx, coredata.OrphanMaintenanceCandidateRequest{
		OlderThan:   p.cutoff,
		AfterCursor: cursor.RootID,
		Limit:       limit,
	})
	if err != nil {
		return nil, err
	}
	result := make([]conversationCleanupCandidate, 0, len(rows))
	for _, row := range rows {
		result = append(result, conversationCleanupCandidate{
			RootID: row.CursorID, ActivityAt: p.cutoff,
			RuleID: row.RuleID, Table: row.Table, RecordID: row.RecordID,
			ReferenceTable: row.ReferenceTable, ReferenceID: row.ReferenceID,
			OrphanAction: row.Action, ObservedAt: row.ObservedAt,
		})
	}
	return result, nil
}

func (p *orphanReportPolicy) ProcessCandidate(ctx context.Context, candidate conversationCleanupCandidate) (conversationCleanupOutcome, error) {
	if candidate.RuleID == "" || candidate.RecordID == "" {
		return conversationCleanupOutcome{}, fmt.Errorf("invalid orphan report candidate")
	}
	if !p.mutate {
		return conversationCleanupOutcome{
			Eligible: true,
			Reason:   fmt.Sprintf("orphan_%s:%s", candidate.OrphanAction, candidate.RuleID),
		}, nil
	}
	result, err := p.data.MaintainOrphanCandidate(ctx, coredata.OrphanMaintenanceRequest{
		RuleID: candidate.RuleID, RecordID: candidate.RecordID, OlderThan: p.cutoff,
		Lease: conversationCleanupLease(ctx),
	})
	if err != nil {
		return conversationCleanupOutcome{}, err
	}
	if result == nil {
		return conversationCleanupOutcome{}, fmt.Errorf("orphan cleanup returned no result for rule=%q record=%q", candidate.RuleID, candidate.RecordID)
	}
	return conversationCleanupOutcome{
		Eligible: result.Eligible,
		Deleted:  result.Deleted,
		Mutated:  result.Mutated,
		Reason:   fmt.Sprintf("orphan_%s:%s:%s", result.Action, candidate.RuleID, result.Reason),
	}, nil
}
