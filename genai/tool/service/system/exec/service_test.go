package exec

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestServiceExecuteReturnsErrorForNonZeroExit(t *testing.T) {
	svc := New()
	in := &Input{
		Commands: []string{"false"},
	}
	out := &Output{}

	err := svc.Execute(context.Background(), in, out)
	if err == nil {
		t.Fatalf("expected non-nil error for non-zero command exit")
	}
}

func TestServiceExecuteReturnsContextErrorOnDeadline(t *testing.T) {
	svc := New()
	in := &Input{
		Commands:  []string{"sleep 10"},
		TimeoutMs: 10000,
	}
	out := &Output{}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := svc.Execute(ctx, in, out)
	if err == nil {
		t.Fatalf("expected context error on deadline")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) &&
		!strings.Contains(strings.ToLower(err.Error()), "cancel") &&
		!strings.Contains(strings.ToLower(err.Error()), "timed out") {
		t.Fatalf("expected context deadline/canceled error, got: %v", err)
	}
}
