package agently

import (
	"context"
	"fmt"
	"time"

	coredata "github.com/viant/agently-core/app/store/data"
)

type technicalCleanupPolicy struct {
	data        conversationCleanupData
	scope       coredata.TechnicalMaintenanceScope
	retention   time.Duration
	mode        coredata.ConversationMaintenanceMode
	now         func() time.Time
	cutoff      time.Time
	evaluatedAt time.Time
}

func newTechnicalCleanupPolicy(data conversationCleanupData, scope coredata.TechnicalMaintenanceScope, retention time.Duration, deleteEnabled bool) *technicalCleanupPolicy {
	if retention <= 0 {
		retention = 30 * 24 * time.Hour
	}
	mode := coredata.ConversationMaintenanceDryRun
	if deleteEnabled {
		mode = coredata.ConversationMaintenanceDelete
	}
	return &technicalCleanupPolicy{
		data: data, scope: scope, retention: retention, mode: mode,
		now: func() time.Time { return time.Now().UTC() },
	}
}

func (p *technicalCleanupPolicy) Name() string {
	mode := "dry_run"
	if p != nil && p.mode == coredata.ConversationMaintenanceDelete {
		mode = "delete"
	}
	scope := coredata.TechnicalMaintenanceScope("")
	if p != nil {
		scope = p.scope
	}
	return fmt.Sprintf("technical_retention_%s_%s", scope, mode)
}

func (p *technicalCleanupPolicy) SelectCandidates(ctx context.Context, cursor conversationCleanupCursor, limit int) ([]conversationCleanupCandidate, error) {
	if p == nil || p.data == nil {
		return nil, fmt.Errorf("conversation cleanup data service is unavailable")
	}
	if cursor.RootID == "" {
		p.evaluatedAt = p.now().UTC()
		p.cutoff = p.evaluatedAt.Add(-p.retention)
	}
	rows, err := p.data.ListTechnicalMaintenanceCandidates(ctx, coredata.TechnicalMaintenanceCandidateRequest{
		Scope: p.scope, OlderThan: p.cutoff, EvaluatedAt: p.evaluatedAt,
		AfterCursor: cursor.RootID, Limit: limit,
	})
	if err != nil {
		return nil, err
	}
	result := make([]conversationCleanupCandidate, 0, len(rows))
	for _, row := range rows {
		result = append(result, conversationCleanupCandidate{
			RootID: row.CursorID, ActivityAt: p.evaluatedAt,
			RecordID: row.RecordID, TechnicalKind: row.Kind, TechnicalScope: row.Scope,
			ObservedAt: row.ObservedAt,
		})
	}
	return result, nil
}

func (p *technicalCleanupPolicy) ProcessCandidate(ctx context.Context, candidate conversationCleanupCandidate) (conversationCleanupOutcome, error) {
	if p == nil || p.data == nil {
		return conversationCleanupOutcome{}, fmt.Errorf("conversation cleanup data service is unavailable")
	}
	if candidate.TechnicalKind == "" || candidate.RecordID == "" || candidate.TechnicalScope != p.scope {
		return conversationCleanupOutcome{}, fmt.Errorf("invalid technical cleanup candidate")
	}
	result, err := p.data.MaintainTechnicalCandidate(ctx, coredata.TechnicalMaintenanceRequest{
		Kind: candidate.TechnicalKind, Scope: p.scope, RecordID: candidate.RecordID,
		OlderThan: p.cutoff, EvaluatedAt: p.evaluatedAt, Mode: p.mode,
		Lease: conversationCleanupLease(ctx),
	})
	if err != nil {
		return conversationCleanupOutcome{}, err
	}
	if result == nil {
		return conversationCleanupOutcome{}, fmt.Errorf("technical cleanup returned no result for kind=%q record=%q", candidate.TechnicalKind, candidate.RecordID)
	}
	return conversationCleanupOutcome{
		Eligible: result.Eligible,
		Deleted:  result.Deleted,
		Reason:   fmt.Sprintf("technical_%s:%s", result.Kind, result.Reason),
	}, nil
}
