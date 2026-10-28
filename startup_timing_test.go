package agently

import (
	"context"
	"testing"
	"time"

	"github.com/viant/agently-core/protocol/primitive/observe"
)

func TestBackendPrimitiveTraceIsOnlyRuntimeSwitch(t *testing.T) {
	t.Setenv("AGENTLY_PRIMITIVE_TRACE", "1")
	t.Setenv("BACKEND_PRIMITIVE_TRACE", "")
	ctx, finish := startupTiming(context.Background(), time.Now())
	defer finish("test")
	if observe.Enabled(ctx) {
		t.Fatal("legacy trace switch remained active")
	}
	t.Setenv("BACKEND_PRIMITIVE_TRACE", "1")
	active, done := startupTiming(context.Background(), time.Now())
	defer done("test")
	if !observe.Enabled(active) {
		t.Fatal("backend trace switch did not activate")
	}
}
