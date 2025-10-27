package executil

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestResolveToolStatus_DataDriven(t *testing.T) {
	type testCase struct {
		name     string
		err      error
		parentFn func() context.Context
		expected string
	}
	cases := []testCase{
		{name: "success", err: nil, parentFn: context.Background, expected: "completed"},
		{name: "canceled-by-parent", err: nil, parentFn: func() context.Context { ctx, cancel := context.WithCancel(context.Background()); cancel(); return ctx }, expected: "canceled"},
		{name: "exec-error", err: assert.AnError, parentFn: context.Background, expected: "failed"},
		{name: "exec-canceled", err: context.Canceled, parentFn: context.Background, expected: "canceled"},
		{name: "exec-deadline", err: context.DeadlineExceeded, parentFn: context.Background, expected: "canceled"},
	}
	for _, tc := range cases {
		ctx := tc.parentFn()
		got, _ := resolveToolStatus(tc.err, ctx)
		assert.EqualValues(t, tc.expected, got, tc.name)
	}
}

func TestToolExecContext_Timeout(t *testing.T) {
	// 50ms timeout
	_ = os.Setenv("AGENTLY_TOOLCALL_TIMEOUT", "50ms")
	defer os.Unsetenv("AGENTLY_TOOLCALL_TIMEOUT")
	ctx := context.Background()
	execCtx, cancel := toolExecContext(ctx)
	defer cancel()
	select {
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected timeout before 200ms")
	case <-execCtx.Done():
		// expected
		assert.Error(t, execCtx.Err())
	}
}
