package agently

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	agentlyrt "github.com/viant/agently/runtime"
)

func newTestConversationCleanupWorker(options agentlyrt.ConversationCleanupOptions, policies ...conversationCleanupPolicy) *conversationCleanupWorker {
	return newConversationCleanupWorker(&recordingConversationCleanupData{}, options, policies...)
}

func TestConversationCleanupWorkerDisabledDoesNotCallPolicy(t *testing.T) {
	policy := &recordingConversationCleanupPolicy{}
	worker := newTestConversationCleanupWorker(agentlyrt.ConversationCleanupOptions{
		Enabled:   false,
		BatchSize: 10,
	}, policy)

	result, err := worker.runPass(context.Background())
	if err != nil {
		t.Fatalf("runPass() error: %v", err)
	}
	if !reflect.DeepEqual(result, conversationCleanupResult{}) {
		t.Fatalf("result = %#v", result)
	}
	if policy.selectCalls != 0 || len(policy.processed) != 0 {
		t.Fatalf("disabled worker called policy: selects=%d processed=%v", policy.selectCalls, policy.processed)
	}
}

func TestConversationCleanupWorkerPaginatesAndContinuesAfterCandidateFailure(t *testing.T) {
	base := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	policy := &recordingConversationCleanupPolicy{
		pages: [][]conversationCleanupCandidate{
			{{RootID: "a", ActivityAt: base}},
			{{RootID: "b", ActivityAt: base.Add(time.Minute)}},
			{{RootID: "c", ActivityAt: base.Add(2 * time.Minute)}},
			{{RootID: "d", ActivityAt: base.Add(3 * time.Minute)}},
		},
		failures: map[string]error{"a": errors.New("broken graph")},
		outcomes: map[string]conversationCleanupOutcome{
			"b": {Reason: "recent_activity"},
			"c": {Eligible: true, Reason: "eligible"},
			"d": {Eligible: true, Reason: "eligible"},
		},
	}
	worker := newTestConversationCleanupWorker(agentlyrt.ConversationCleanupOptions{
		Enabled:   true,
		BatchSize: 1,
		Timeout:   time.Second,
	}, policy)

	result, err := worker.runPass(context.Background())
	if err != nil {
		t.Fatalf("runPass() error: %v", err)
	}
	if result.Scanned != 3 || result.Eligible != 1 || result.Deleted != 0 || result.Skipped != 1 || result.Failed != 1 {
		t.Fatalf("result = %#v", result)
	}
	if want := map[string]int{"error": 1, "recent_activity": 1, "eligible": 1}; !reflect.DeepEqual(result.Reasons, want) {
		t.Fatalf("reasons = %#v, want %#v", result.Reasons, want)
	}
	if want := []string{"a", "b", "c"}; !reflect.DeepEqual(policy.processed, want) {
		t.Fatalf("processed = %v, want %v", policy.processed, want)
	}
	if len(policy.cursors) != 3 || policy.cursors[1].RootID != "a" || policy.cursors[2].RootID != "b" {
		t.Fatalf("cursors = %#v", policy.cursors)
	}
}

func TestFormatConversationCleanupReasonsIsStable(t *testing.T) {
	got := formatConversationCleanupReasons(map[string]int{"recent_activity": 3, "eligible": 2})
	if got != "eligible:2,recent_activity:3" {
		t.Fatalf("formatConversationCleanupReasons() = %q", got)
	}
}

func TestConversationCleanupWorkerTimeoutBoundsPass(t *testing.T) {
	policy := &blockingConversationCleanupPolicy{}
	worker := newTestConversationCleanupWorker(agentlyrt.ConversationCleanupOptions{
		Enabled:   true,
		BatchSize: 1,
		Timeout:   30 * time.Millisecond,
	}, policy)

	started := time.Now()
	_, err := worker.runPass(context.Background())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline, got %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("timeout was not applied, elapsed=%s", elapsed)
	}
}

func TestConversationCleanupWorkerUsesIndependentPolicyTimeouts(t *testing.T) {
	blocking := &blockingConversationCleanupPolicy{}
	next := &recordingConversationCleanupPolicy{
		name:  "next",
		pages: [][]conversationCleanupCandidate{{{RootID: "eligible", ActivityAt: time.Now()}}},
		outcomes: map[string]conversationCleanupOutcome{
			"eligible": {Eligible: true, Reason: "eligible"},
		},
	}
	worker := newTestConversationCleanupWorker(agentlyrt.ConversationCleanupOptions{
		Enabled:   true,
		BatchSize: 1,
		Timeout:   30 * time.Millisecond,
	}, blocking, next)

	result, err := worker.runPass(context.Background())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected policy timeout, got %v", err)
	}
	if result.Eligible != 1 || !reflect.DeepEqual(next.processed, []string{"eligible"}) {
		t.Fatalf("next policy did not run after timeout: result=%#v processed=%v", result, next.processed)
	}
	if len(result.PolicyResults) != 2 || result.PolicyResults[0].Name != "blocking" || result.PolicyResults[1].Name != "next" {
		t.Fatalf("policy results = %#v", result.PolicyResults)
	}
	if !errors.Is(result.PolicyResults[0].Err, context.DeadlineExceeded) || result.PolicyResults[1].Err != nil {
		t.Fatalf("policy errors = %#v", result.PolicyResults)
	}
}

func TestConversationCleanupWorkerContinuesAfterPolicyError(t *testing.T) {
	broken := &failingConversationCleanupPolicy{name: "broken", err: errors.New("select failed")}
	next := &recordingConversationCleanupPolicy{
		name:  "next",
		pages: [][]conversationCleanupCandidate{{{RootID: "eligible", ActivityAt: time.Now()}}},
		outcomes: map[string]conversationCleanupOutcome{
			"eligible": {Eligible: true, Reason: "eligible"},
		},
	}
	worker := newTestConversationCleanupWorker(agentlyrt.ConversationCleanupOptions{
		Enabled:   true,
		BatchSize: 1,
		Timeout:   time.Second,
	}, broken, next)

	result, err := worker.runPass(context.Background())
	if err == nil || !strings.Contains(err.Error(), "cleanup policy broken: select failed") {
		t.Fatalf("runPass() error = %v", err)
	}
	if result.Eligible != 1 || !reflect.DeepEqual(next.processed, []string{"eligible"}) {
		t.Fatalf("next policy did not run after error: result=%#v processed=%v", result, next.processed)
	}
}

func TestConversationCleanupWorkerAppliesBatchPerPolicy(t *testing.T) {
	first := eligibleConversationCleanupPolicy("first", "first-1", "first-2")
	second := eligibleConversationCleanupPolicy("second", "second-1", "second-2")
	worker := newTestConversationCleanupWorker(agentlyrt.ConversationCleanupOptions{
		Enabled:   true,
		BatchSize: 1,
		Timeout:   time.Second,
	}, first, second)

	result, err := worker.runPass(context.Background())
	if err != nil {
		t.Fatalf("runPass() error: %v", err)
	}
	if result.Eligible != 2 || !reflect.DeepEqual(first.processed, []string{"first-1"}) || !reflect.DeepEqual(second.processed, []string{"second-1"}) {
		t.Fatalf("batch was not applied per policy: result=%#v first=%v second=%v", result, first.processed, second.processed)
	}
}

func TestConversationCleanupWorkerStopsWhenParentContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	policy := eligibleConversationCleanupPolicy("unused", "candidate")
	worker := newTestConversationCleanupWorker(agentlyrt.ConversationCleanupOptions{
		Enabled:   true,
		BatchSize: 1,
		Timeout:   time.Second,
	}, policy)

	result, err := worker.runPass(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runPass() error = %v", err)
	}
	if len(result.PolicyResults) != 0 || len(policy.processed) != 0 {
		t.Fatalf("canceled pass ran policies: result=%#v processed=%v", result, policy.processed)
	}
}

func TestConversationCleanupWorkerSkipsOverlappingLocalPass(t *testing.T) {
	policy := newHeldConversationCleanupPolicy()
	worker := newTestConversationCleanupWorker(agentlyrt.ConversationCleanupOptions{
		Enabled:   true,
		BatchSize: 1,
		Timeout:   time.Second,
	}, policy)

	firstDone := make(chan error, 1)
	go func() {
		_, err := worker.runPass(context.Background())
		firstDone <- err
	}()
	select {
	case <-policy.entered:
	case <-time.After(time.Second):
		t.Fatal("first pass did not enter policy")
	}

	result, err := worker.runPass(context.Background())
	if err != nil {
		t.Fatalf("overlapping runPass() error: %v", err)
	}
	if !result.OverlapSkipped {
		t.Fatalf("overlapping result = %#v", result)
	}
	close(policy.release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first runPass() error: %v", err)
	}
}

func TestConversationCleanupWorkerSkipsPassHeldByAnotherWorker(t *testing.T) {
	data := &recordingConversationCleanupData{}
	held := newHeldConversationCleanupPolicy()
	first := newConversationCleanupWorker(data, agentlyrt.ConversationCleanupOptions{
		Enabled: true, BatchSize: 1, Timeout: time.Second,
	}, held)
	secondPolicy := eligibleConversationCleanupPolicy("second", "candidate")
	second := newConversationCleanupWorker(data, agentlyrt.ConversationCleanupOptions{
		Enabled: true, BatchSize: 1, Timeout: time.Second,
	}, secondPolicy)

	firstDone := make(chan error, 1)
	go func() {
		_, err := first.runPass(context.Background())
		firstDone <- err
	}()
	select {
	case <-held.entered:
	case <-time.After(time.Second):
		t.Fatal("first worker did not enter policy")
	}

	result, err := second.runPass(context.Background())
	if err != nil {
		t.Fatalf("second runPass() error: %v", err)
	}
	if !result.LeaseSkipped || result.OverlapSkipped {
		t.Fatalf("second result = %#v", result)
	}
	if secondPolicy.selectCalls != 0 {
		t.Fatalf("second worker ran policy %d times", secondPolicy.selectCalls)
	}

	close(held.release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first runPass() error: %v", err)
	}
}

func TestConversationCleanupWorkerRenewsLeaseDuringLongPass(t *testing.T) {
	data := &recordingConversationCleanupData{}
	held := newHeldConversationCleanupPolicy()
	worker := newConversationCleanupWorker(data, agentlyrt.ConversationCleanupOptions{
		Enabled: true, BatchSize: 1, Timeout: time.Second,
	}, held)
	worker.leaseTTL = 200 * time.Millisecond
	worker.leaseRenewInterval = 20 * time.Millisecond

	done := make(chan error, 1)
	go func() {
		_, err := worker.runPass(context.Background())
		done <- err
	}()
	select {
	case <-held.entered:
	case <-time.After(time.Second):
		t.Fatal("worker did not enter policy")
	}
	deadline := time.Now().Add(time.Second)
	for {
		data.leaseMu.Lock()
		renewals := data.leaseRenewals
		data.leaseMu.Unlock()
		if renewals > 0 {
			break
		}
		if !time.Now().Before(deadline) {
			t.Fatal("worker did not renew lease")
		}
		time.Sleep(5 * time.Millisecond)
	}

	close(held.release)
	if err := <-done; err != nil {
		t.Fatalf("runPass() error: %v", err)
	}
}

func TestConversationCleanupWorkerStopsPoliciesWhenLeaseRenewalFails(t *testing.T) {
	data := &recordingConversationCleanupData{leaseRenewError: errors.New("renew unavailable")}
	blocking := &blockingConversationCleanupPolicy{}
	next := eligibleConversationCleanupPolicy("next", "candidate")
	worker := newConversationCleanupWorker(data, agentlyrt.ConversationCleanupOptions{
		Enabled: true, BatchSize: 1, Timeout: time.Second,
	}, blocking, next)
	worker.leaseTTL = 200 * time.Millisecond
	worker.leaseRenewInterval = 20 * time.Millisecond

	_, err := worker.runPass(context.Background())
	if err == nil || !strings.Contains(err.Error(), "renew conversation cleanup lease: renew unavailable") {
		t.Fatalf("runPass() error = %v", err)
	}
	if next.selectCalls != 0 {
		t.Fatalf("worker continued after renewal failure; next selects=%d", next.selectCalls)
	}
}

type recordingConversationCleanupPolicy struct {
	name        string
	selectCalls int
	pages       [][]conversationCleanupCandidate
	cursors     []conversationCleanupCursor
	processed   []string
	failures    map[string]error
	outcomes    map[string]conversationCleanupOutcome
}

func (p *recordingConversationCleanupPolicy) Name() string {
	if p.name != "" {
		return p.name
	}
	return "recording"
}

func (p *recordingConversationCleanupPolicy) SelectCandidates(_ context.Context, cursor conversationCleanupCursor, _ int) ([]conversationCleanupCandidate, error) {
	p.cursors = append(p.cursors, cursor)
	index := p.selectCalls
	p.selectCalls++
	if index >= len(p.pages) {
		return nil, nil
	}
	return p.pages[index], nil
}

func (p *recordingConversationCleanupPolicy) ProcessCandidate(_ context.Context, candidate conversationCleanupCandidate) (conversationCleanupOutcome, error) {
	p.processed = append(p.processed, candidate.RootID)
	if err := p.failures[candidate.RootID]; err != nil {
		return conversationCleanupOutcome{}, err
	}
	return p.outcomes[candidate.RootID], nil
}

type blockingConversationCleanupPolicy struct{}

func (p *blockingConversationCleanupPolicy) Name() string { return "blocking" }

func (p *blockingConversationCleanupPolicy) SelectCandidates(ctx context.Context, _ conversationCleanupCursor, _ int) ([]conversationCleanupCandidate, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (p *blockingConversationCleanupPolicy) ProcessCandidate(context.Context, conversationCleanupCandidate) (conversationCleanupOutcome, error) {
	return conversationCleanupOutcome{}, nil
}

type heldConversationCleanupPolicy struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func newHeldConversationCleanupPolicy() *heldConversationCleanupPolicy {
	return &heldConversationCleanupPolicy{entered: make(chan struct{}), release: make(chan struct{})}
}

func (p *heldConversationCleanupPolicy) Name() string { return "held" }

func (p *heldConversationCleanupPolicy) SelectCandidates(context.Context, conversationCleanupCursor, int) ([]conversationCleanupCandidate, error) {
	p.once.Do(func() { close(p.entered) })
	<-p.release
	return nil, nil
}

func (p *heldConversationCleanupPolicy) ProcessCandidate(context.Context, conversationCleanupCandidate) (conversationCleanupOutcome, error) {
	return conversationCleanupOutcome{}, nil
}

type failingConversationCleanupPolicy struct {
	name string
	err  error
}

func (p *failingConversationCleanupPolicy) Name() string { return p.name }

func (p *failingConversationCleanupPolicy) SelectCandidates(context.Context, conversationCleanupCursor, int) ([]conversationCleanupCandidate, error) {
	return nil, p.err
}

func (p *failingConversationCleanupPolicy) ProcessCandidate(context.Context, conversationCleanupCandidate) (conversationCleanupOutcome, error) {
	return conversationCleanupOutcome{}, nil
}

func eligibleConversationCleanupPolicy(name string, candidateIDs ...string) *recordingConversationCleanupPolicy {
	candidates := make([]conversationCleanupCandidate, 0, len(candidateIDs))
	outcomes := make(map[string]conversationCleanupOutcome, len(candidateIDs))
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, candidateID := range candidateIDs {
		candidates = append(candidates, conversationCleanupCandidate{RootID: candidateID, ActivityAt: base.Add(time.Duration(i) * time.Second)})
		outcomes[candidateID] = conversationCleanupOutcome{Eligible: true, Reason: "eligible"}
	}
	return &recordingConversationCleanupPolicy{name: name, pages: [][]conversationCleanupCandidate{candidates}, outcomes: outcomes}
}
