package tool

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"
	"time"
)

func TestWaitDiscoveryStage_NoHeartbeatBeforeInterval(t *testing.T) {
	t.Setenv("AGENTLY_SCHEDULER_DEBUG", "1")
	buf := captureDiscoveryWaitLogs(t)
	reg := &Registry{discoveryWaitEvery: 80 * time.Millisecond}

	err := reg.waitDiscoveryStage(context.Background(), "operation", "wait_test_fast", func(ctx context.Context) error {
		time.Sleep(15 * time.Millisecond)
		return nil
	})
	if err != nil {
		t.Fatalf("waitDiscoveryStage returned error: %v", err)
	}

	if strings.Contains(buf.String(), "stage=wait_test_fast") {
		t.Fatalf("unexpected heartbeat log for fast call: %s", buf.String())
	}
}

func TestWaitDiscoveryStage_EmitsFirstHeartbeatThenThrottles(t *testing.T) {
	t.Setenv("AGENTLY_SCHEDULER_DEBUG", "1")
	buf := captureDiscoveryWaitLogs(t)
	reg := &Registry{discoveryWaitEvery: 25 * time.Millisecond}

	err := reg.waitDiscoveryStage(context.Background(), "operation", "wait_test_slow", func(ctx context.Context) error {
		time.Sleep(90 * time.Millisecond)
		return nil
	})
	if err != nil {
		t.Fatalf("waitDiscoveryStage returned error: %v", err)
	}

	count := strings.Count(buf.String(), "stage=wait_test_slow")
	if count != 1 {
		t.Fatalf("expected exactly 1 heartbeat log due to throttle, got %d; logs: %s", count, buf.String())
	}
}

func TestWaitDiscoveryStage_LegacyWhenSchedulerDebugDisabled(t *testing.T) {
	t.Setenv("AGENTLY_SCHEDULER_DEBUG", "")
	buf := captureDiscoveryWaitLogs(t)
	reg := &Registry{discoveryWaitEvery: 25 * time.Millisecond}

	err := reg.waitDiscoveryStage(context.Background(), "operation", "wait_test_legacy", func(ctx context.Context) error {
		time.Sleep(90 * time.Millisecond)
		return nil
	})
	if err != nil {
		t.Fatalf("waitDiscoveryStage returned error: %v", err)
	}

	if strings.Contains(buf.String(), "stage=wait_test_legacy") {
		t.Fatalf("unexpected wait log in legacy mode: %s", buf.String())
	}
}

func TestWaitDiscoveryStage_ReturnsOnContextDeadlineEvenIfStageIgnoresContext(t *testing.T) {
	t.Setenv("AGENTLY_SCHEDULER_DEBUG", "1")
	reg := &Registry{discoveryWaitEvery: 10 * time.Millisecond}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	block := make(chan struct{})
	defer close(block)

	errCh := make(chan error, 1)
	go func() {
		errCh <- reg.waitDiscoveryStage(ctx, "guardian", "wait_test_ctx_done", func(context.Context) error {
			<-block
			return nil
		})
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected deadline exceeded, got %v", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("waitDiscoveryStage did not return promptly on context deadline")
	}
}

func TestWaitDiscoveryStage_CancelsChildContextOnParentCancel(t *testing.T) {
	t.Setenv("AGENTLY_SCHEDULER_DEBUG", "1")
	reg := &Registry{discoveryWaitEvery: 10 * time.Millisecond}

	parentCtx, parentCancel := context.WithCancel(context.Background())
	defer parentCancel()

	childDone := make(chan error, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- reg.waitDiscoveryStage(parentCtx, "guardian", "wait_test_child_cancel", func(ctx context.Context) error {
			<-ctx.Done()
			childDone <- ctx.Err()
			return ctx.Err()
		})
	}()

	time.Sleep(20 * time.Millisecond)
	parentCancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context canceled, got %v", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("waitDiscoveryStage did not return promptly on parent cancel")
	}

	select {
	case err := <-childDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected child context canceled, got %v", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("child discovery context was not canceled")
	}
}

func captureDiscoveryWaitLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prevOut := log.Writer()
	prevFlags := log.Flags()
	prevPrefix := log.Prefix()
	log.SetOutput(&buf)
	log.SetFlags(0)
	log.SetPrefix("")
	t.Cleanup(func() {
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
		log.SetPrefix(prevPrefix)
	})
	return &buf
}
