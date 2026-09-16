package agently

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	coredata "github.com/viant/agently-core/app/store/data"
	agentlyrt "github.com/viant/agently/runtime"
)

type conversationCleanupData interface {
	ListConversationMaintenanceCandidates(context.Context, coredata.ConversationMaintenanceCandidateRequest) ([]coredata.ConversationMaintenanceCandidate, error)
	MaintainConversationTree(context.Context, coredata.ConversationMaintenanceRequest) (*coredata.ConversationMaintenanceResult, error)
	ListScheduledRunMaintenanceCandidates(context.Context, coredata.ScheduledRunMaintenanceCandidateRequest) ([]coredata.ScheduledRunMaintenanceCandidate, error)
	MaintainScheduledRun(context.Context, coredata.ScheduledRunMaintenanceRequest) (*coredata.ScheduledRunMaintenanceResult, error)
	ListOrphanMaintenanceCandidates(context.Context, coredata.OrphanMaintenanceCandidateRequest) ([]coredata.OrphanMaintenanceCandidate, error)
	MaintainOrphanCandidate(context.Context, coredata.OrphanMaintenanceRequest) (*coredata.OrphanMaintenanceResult, error)
	ListTechnicalMaintenanceCandidates(context.Context, coredata.TechnicalMaintenanceCandidateRequest) ([]coredata.TechnicalMaintenanceCandidate, error)
	MaintainTechnicalCandidate(context.Context, coredata.TechnicalMaintenanceRequest) (*coredata.TechnicalMaintenanceResult, error)
	AcquireMaintenanceLease(context.Context, coredata.MaintenanceLeaseAcquireRequest) (*coredata.MaintenanceLeaseAcquireResult, error)
	RenewMaintenanceLease(context.Context, coredata.MaintenanceLease, time.Duration) (*coredata.MaintenanceLeaseRenewResult, error)
	ReleaseMaintenanceLease(context.Context, coredata.MaintenanceLease) (bool, error)
	DeleteExpiredMaintenanceLeases(context.Context, coredata.MaintenanceLease) (int64, error)
}

const (
	conversationCleanupLeaseKey            = "conversation_cleanup"
	conversationCleanupLeaseTTL            = 2 * time.Minute
	conversationCleanupLeaseRenewInterval  = 30 * time.Second
	conversationCleanupLeaseReleaseTimeout = 10 * time.Second
)

type conversationCleanupLeaseContextKey struct{}

func conversationCleanupLease(ctx context.Context) coredata.MaintenanceLease {
	lease, _ := ctx.Value(conversationCleanupLeaseContextKey{}).(coredata.MaintenanceLease)
	return lease
}

type conversationCleanupCursor struct {
	ActivityAt time.Time
	RootID     string
}

type conversationCleanupCandidate struct {
	RootID          string
	ExpectedOwnerID string
	ActivityAt      time.Time
	RuleID          string
	Table           string
	RecordID        string
	ReferenceTable  string
	ReferenceID     string
	OrphanAction    coredata.OrphanMaintenanceAction
	TechnicalKind   coredata.TechnicalMaintenanceKind
	TechnicalScope  coredata.TechnicalMaintenanceScope
	ObservedAt      time.Time
}

func (c conversationCleanupCandidate) cursor() conversationCleanupCursor {
	return conversationCleanupCursor{ActivityAt: c.ActivityAt, RootID: c.RootID}
}

type conversationCleanupOutcome struct {
	Eligible bool
	Deleted  bool
	Mutated  bool
	Reason   string
}

type conversationCleanupPolicy interface {
	Name() string
	SelectCandidates(context.Context, conversationCleanupCursor, int) ([]conversationCleanupCandidate, error)
	ProcessCandidate(context.Context, conversationCleanupCandidate) (conversationCleanupOutcome, error)
}

type conversationCleanupResult struct {
	Scanned              int
	Eligible             int
	Deleted              int
	Mutated              int
	Skipped              int
	Failed               int
	OverlapSkipped       bool
	LeaseSkipped         bool
	LeaseOwnerID         string
	LeaseUntil           time.Time
	ExpiredLeasesDeleted int64
	Reasons              map[string]int
	PolicyResults        []conversationCleanupPolicyResult
}

type conversationCleanupPolicyResult struct {
	Name     string
	Duration time.Duration
	Result   conversationCleanupResult
	Err      error
}

func (r *conversationCleanupResult) add(other conversationCleanupResult) {
	r.Scanned += other.Scanned
	r.Eligible += other.Eligible
	r.Deleted += other.Deleted
	r.Mutated += other.Mutated
	r.Skipped += other.Skipped
	r.Failed += other.Failed
	r.OverlapSkipped = r.OverlapSkipped || other.OverlapSkipped
	for reason, count := range other.Reasons {
		if r.Reasons == nil {
			r.Reasons = map[string]int{}
		}
		r.Reasons[reason] += count
	}
}

type conversationCleanupWorker struct {
	data               conversationCleanupData
	options            agentlyrt.ConversationCleanupOptions
	policies           []conversationCleanupPolicy
	leaseOwnerID       string
	leaseTTL           time.Duration
	leaseRenewInterval time.Duration
	running            atomic.Bool
}

func newConversationCleanupWorker(data conversationCleanupData, options agentlyrt.ConversationCleanupOptions, policies ...conversationCleanupPolicy) *conversationCleanupWorker {
	return &conversationCleanupWorker{
		data:               data,
		options:            options,
		policies:           policies,
		leaseOwnerID:       newConversationCleanupLeaseOwnerID(),
		leaseTTL:           conversationCleanupLeaseTTL,
		leaseRenewInterval: conversationCleanupLeaseRenewInterval,
	}
}

func startConversationCleanup(ctx context.Context, dataSvc conversationCleanupData, options agentlyrt.ConversationCleanupOptions) {
	if !options.Enabled {
		return
	}
	policies := conversationCleanupPolicies(dataSvc, options)
	if len(policies) == 0 {
		log.Printf("conversation cleanup: worker enabled, no cleanup policies enabled interactive_mode=%s scheduled_mode=%s orphan_mode=%s",
			options.InteractiveMode, options.ScheduledMode, options.OrphanMode)
		return
	}
	newConversationCleanupWorker(dataSvc, options, policies...).start(ctx)
}

func conversationCleanupPolicies(dataSvc conversationCleanupData, options agentlyrt.ConversationCleanupOptions) []conversationCleanupPolicy {
	var result []conversationCleanupPolicy
	if options.InteractiveMode.Enabled() {
		result = append(result, newInteractiveConversationCleanupPolicy(dataSvc, options.InteractiveRetention, options.InteractiveMode.Executes()))
		result = append(result, newTechnicalCleanupPolicy(dataSvc, coredata.TechnicalMaintenanceInteractive, options.InteractiveRetention, options.InteractiveMode.Executes()))
	}
	if options.ScheduledMode.Enabled() {
		result = append(result, newScheduledRunCleanupPolicy(dataSvc, options.ScheduledRetention, options.ScheduledMode.Executes()))
		result = append(result, newScheduledConversationFallbackPolicy(dataSvc, options.ScheduledRetention, options.ScheduledMode.Executes()))
		result = append(result, newTechnicalCleanupPolicy(dataSvc, coredata.TechnicalMaintenanceScheduled, options.ScheduledRetention, options.ScheduledMode.Executes()))
	}
	if options.OrphanMode.Enabled() {
		technicalRetention := options.InteractiveRetention
		if options.ScheduledRetention > technicalRetention {
			technicalRetention = options.ScheduledRetention
		}
		result = append(result, newTechnicalCleanupPolicy(dataSvc, coredata.TechnicalMaintenanceUnclassified, technicalRetention, options.OrphanMode.Executes()))
		result = append(result, newOrphanMaintenancePolicy(dataSvc, options.OrphanMinAge, options.OrphanMode.Executes()))
	}
	return result
}

func (w *conversationCleanupWorker) start(ctx context.Context) {
	if w == nil || !w.options.Enabled || len(w.policies) == 0 {
		return
	}
	log.Printf("conversation cleanup: worker started interval=%s batch_size_per_policy=%d timeout_per_policy=%s run_on_start=%t debug=%t interactive_mode=%s interactive_retention=%s scheduled_mode=%s scheduled_retention=%s orphan_mode=%s orphan_min_age=%s policies=%s coordination=database lease_key=%s lease_owner=%s lease_ttl=%s lease_renew_interval=%s",
		w.options.Interval, w.options.BatchSize, w.options.Timeout, w.options.RunOnStart, w.options.Debug,
		w.options.InteractiveMode, w.options.InteractiveRetention,
		w.options.ScheduledMode, w.options.ScheduledRetention,
		w.options.OrphanMode, w.options.OrphanMinAge,
		strings.Join(w.policyNames(), ","), conversationCleanupLeaseKey, w.leaseOwnerID, w.leaseTTL, w.leaseRenewInterval)
	go w.loop(ctx)
}

func (w *conversationCleanupWorker) loop(ctx context.Context) {
	if w.options.RunOnStart {
		w.runAndLog(ctx)
	}
	ticker := time.NewTicker(w.options.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.runAndLog(ctx)
		}
	}
}

func (w *conversationCleanupWorker) runAndLog(ctx context.Context) {
	started := time.Now()
	result, err := w.runPass(ctx)
	if result.OverlapSkipped {
		if w.options.Debug {
			log.Printf("conversation cleanup: pass skipped because another local pass is running")
		}
		return
	}
	if result.LeaseSkipped {
		log.Printf("conversation cleanup: pass skipped because distributed lease is held lease_key=%s lease_owner=%s lease_until=%s",
			conversationCleanupLeaseKey, result.LeaseOwnerID, result.LeaseUntil.UTC().Format(time.RFC3339Nano))
		return
	}
	for _, policyResult := range result.PolicyResults {
		policy := policyResult.Result
		log.Printf("conversation cleanup: policy finished policy=%s duration=%s scanned=%d eligible=%d deleted=%d mutated=%d skipped=%d failed=%d reasons=%s err=%v",
			policyResult.Name, policyResult.Duration.Round(time.Millisecond), policy.Scanned, policy.Eligible,
			policy.Deleted, policy.Mutated, policy.Skipped, policy.Failed,
			formatConversationCleanupReasons(policy.Reasons), policyResult.Err)
	}
	log.Printf("conversation cleanup: pass finished duration=%s scanned=%d eligible=%d deleted=%d mutated=%d skipped=%d failed=%d expired_leases_deleted=%d reasons=%s err=%v",
		time.Since(started).Round(time.Millisecond), result.Scanned, result.Eligible, result.Deleted, result.Mutated, result.Skipped, result.Failed, result.ExpiredLeasesDeleted, formatConversationCleanupReasons(result.Reasons), err)
}

func (w *conversationCleanupWorker) runPass(ctx context.Context) (conversationCleanupResult, error) {
	var result conversationCleanupResult
	if w == nil || !w.options.Enabled || len(w.policies) == 0 {
		return result, nil
	}
	if !w.running.CompareAndSwap(false, true) {
		result.OverlapSkipped = true
		return result, nil
	}
	defer w.running.Store(false)
	if w.data == nil {
		return result, fmt.Errorf("conversation cleanup data service is unavailable")
	}

	acquired, err := w.data.AcquireMaintenanceLease(ctx, coredata.MaintenanceLeaseAcquireRequest{
		Key: conversationCleanupLeaseKey, OwnerID: w.leaseOwnerID, TTL: w.leaseTTL,
	})
	if err != nil {
		return result, fmt.Errorf("acquire conversation cleanup lease: %w", err)
	}
	if acquired == nil {
		return result, fmt.Errorf("acquire conversation cleanup lease returned no result")
	}
	result.LeaseOwnerID = acquired.Lease.OwnerID
	result.LeaseUntil = acquired.Lease.LeaseUntil
	if !acquired.Acquired {
		result.LeaseSkipped = true
		return result, nil
	}
	lease := acquired.Lease
	if w.options.Debug {
		log.Printf("conversation cleanup: distributed lease acquired lease_key=%s lease_owner=%s lease_until=%s",
			lease.Key, lease.OwnerID, lease.LeaseUntil.UTC().Format(time.RFC3339Nano))
	}

	leaseCtx, cancelLease := context.WithCancel(ctx)
	heartbeatDone := make(chan error, 1)
	go func() {
		err := w.renewConversationCleanupLease(leaseCtx, lease)
		if err != nil {
			cancelLease()
		}
		heartbeatDone <- err
	}()

	result.ExpiredLeasesDeleted, err = w.data.DeleteExpiredMaintenanceLeases(leaseCtx, lease)
	if err == nil {
		leaseCtx = context.WithValue(leaseCtx, conversationCleanupLeaseContextKey{}, lease)
		result, err = w.runLeasedPass(leaseCtx, result)
	} else {
		err = fmt.Errorf("delete expired maintenance leases: %w", err)
	}
	cancelLease()
	heartbeatErr := <-heartbeatDone
	releaseErr := w.releaseConversationCleanupLease(ctx, lease)
	return result, errors.Join(err, heartbeatErr, releaseErr)
}

func (w *conversationCleanupWorker) runLeasedPass(ctx context.Context, result conversationCleanupResult) (conversationCleanupResult, error) {
	var policyErrors []error
	for _, policy := range w.policies {
		if policy == nil {
			continue
		}
		if err := ctx.Err(); err != nil {
			policyErrors = append(policyErrors, err)
			break
		}
		policyCtx := ctx
		cancel := func() {}
		if w.options.Timeout > 0 {
			policyCtx, cancel = context.WithTimeout(ctx, w.options.Timeout)
		}
		started := time.Now()
		policyResult, err := runConversationCleanupPolicy(policyCtx, policy, w.options.BatchSize, w.options.Debug)
		cancel()
		result.PolicyResults = append(result.PolicyResults, conversationCleanupPolicyResult{
			Name:     policy.Name(),
			Duration: time.Since(started),
			Result:   policyResult,
			Err:      err,
		})
		result.add(policyResult)
		if err != nil {
			policyErrors = append(policyErrors, fmt.Errorf("cleanup policy %s: %w", policy.Name(), err))
			if ctx.Err() != nil {
				break
			}
		}
	}
	return result, errors.Join(policyErrors...)
}

func (w *conversationCleanupWorker) renewConversationCleanupLease(ctx context.Context, lease coredata.MaintenanceLease) error {
	ticker := time.NewTicker(w.leaseRenewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			result, err := w.data.RenewMaintenanceLease(ctx, lease, w.leaseTTL)
			if err != nil {
				return fmt.Errorf("renew conversation cleanup lease: %w", err)
			}
			if result == nil || !result.Renewed {
				return coredata.ErrMaintenanceLeaseLost
			}
			if w.options.Debug {
				log.Printf("conversation cleanup: distributed lease renewed lease_key=%s lease_owner=%s lease_until=%s",
					lease.Key, lease.OwnerID, result.LeaseUntil.UTC().Format(time.RFC3339Nano))
			}
		}
	}
}

func (w *conversationCleanupWorker) releaseConversationCleanupLease(parent context.Context, lease coredata.MaintenanceLease) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), conversationCleanupLeaseReleaseTimeout)
	defer cancel()
	released, err := w.data.ReleaseMaintenanceLease(ctx, lease)
	if err != nil {
		return fmt.Errorf("release conversation cleanup lease: %w", err)
	}
	if !released {
		return coredata.ErrMaintenanceLeaseLost
	}
	if w.options.Debug {
		log.Printf("conversation cleanup: distributed lease released lease_key=%s lease_owner=%s", lease.Key, lease.OwnerID)
	}
	return nil
}

func newConversationCleanupLeaseOwnerID() string {
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		host = "unknown-host"
	}
	return fmt.Sprintf("%s:%d:%s", host, os.Getpid(), uuid.NewString())
}

func runConversationCleanupPolicy(ctx context.Context, policy conversationCleanupPolicy, batchSize int, debug bool) (conversationCleanupResult, error) {
	var result conversationCleanupResult
	if policy == nil || batchSize <= 0 {
		return result, nil
	}
	cursor := conversationCleanupCursor{}
	processedEligible := 0
	for processedEligible < batchSize {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		candidates, err := policy.SelectCandidates(ctx, cursor, batchSize)
		if err != nil {
			return result, err
		}
		if len(candidates) == 0 {
			break
		}
		nextCursor := candidates[len(candidates)-1].cursor()
		if !conversationCleanupCursorAfter(nextCursor, cursor) {
			return result, fmt.Errorf("candidate cursor did not advance: activity_at=%s root_id=%q", nextCursor.ActivityAt.UTC().Format(time.RFC3339Nano), nextCursor.RootID)
		}
		for _, candidate := range candidates {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			if processedEligible >= batchSize {
				break
			}
			result.Scanned++
			outcome, err := policy.ProcessCandidate(ctx, candidate)
			if err != nil {
				result.Failed++
				result.addReason("error")
				if debug {
					log.Printf("conversation cleanup: policy=%s %s failed: %v", policy.Name(), formatConversationCleanupCandidate(candidate), err)
				}
				continue
			}
			if outcome.Eligible {
				result.Eligible++
			}
			if outcome.Deleted {
				result.Deleted++
			}
			if outcome.Mutated {
				result.Mutated++
			}
			if outcome.Eligible || outcome.Deleted || outcome.Mutated {
				processedEligible++
			} else {
				result.Skipped++
			}
			result.addReason(outcome.Reason)
			if debug {
				log.Printf("conversation cleanup: policy=%s %s eligible=%t deleted=%t mutated=%t reason=%s", policy.Name(), formatConversationCleanupCandidate(candidate), outcome.Eligible, outcome.Deleted, outcome.Mutated, outcome.Reason)
			}
		}
		cursor = nextCursor
		if len(candidates) < batchSize {
			break
		}
	}
	return result, nil
}

func formatConversationCleanupCandidate(candidate conversationCleanupCandidate) string {
	if candidate.TechnicalKind != "" {
		return fmt.Sprintf("technical_kind=%s scope=%s record=%s observed_at=%s",
			candidate.TechnicalKind, candidate.TechnicalScope, candidate.RecordID,
			candidate.ObservedAt.UTC().Format(time.RFC3339Nano))
	}
	if candidate.RuleID == "" {
		return fmt.Sprintf("root=%s", candidate.RootID)
	}
	return fmt.Sprintf("rule=%s action=%s table=%s record=%s reference_table=%s reference=%s observed_at=%s",
		candidate.RuleID, candidate.OrphanAction, candidate.Table, candidate.RecordID,
		candidate.ReferenceTable, candidate.ReferenceID, candidate.ObservedAt.UTC().Format(time.RFC3339Nano))
}

func (r *conversationCleanupResult) addReason(reason string) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "unspecified"
	}
	if r.Reasons == nil {
		r.Reasons = map[string]int{}
	}
	r.Reasons[reason]++
}

func formatConversationCleanupReasons(reasons map[string]int) string {
	if len(reasons) == 0 {
		return "none"
	}
	keys := make([]string, 0, len(reasons))
	for reason := range reasons {
		keys = append(keys, reason)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, reason := range keys {
		parts = append(parts, fmt.Sprintf("%s:%d", reason, reasons[reason]))
	}
	return strings.Join(parts, ",")
}

func conversationCleanupCursorAfter(next, previous conversationCleanupCursor) bool {
	if strings.TrimSpace(next.RootID) == "" {
		return false
	}
	if previous.RootID == "" {
		return true
	}
	if next.ActivityAt.After(previous.ActivityAt) {
		return true
	}
	return next.ActivityAt.Equal(previous.ActivityAt) && next.RootID > previous.RootID
}

func (w *conversationCleanupWorker) policyNames() []string {
	result := make([]string, 0, len(w.policies))
	for _, policy := range w.policies {
		if policy == nil {
			continue
		}
		result = append(result, policy.Name())
	}
	return result
}
