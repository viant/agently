package agently

import "testing"

func TestOptionsInitScheduler(t *testing.T) {
	opts := &Options{}
	opts.Init("scheduler")
	if opts.Scheduler == nil {
		t.Fatalf("expected scheduler command to be initialized")
	}
}
