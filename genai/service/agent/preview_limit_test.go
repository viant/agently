package agent

import (
	"testing"

	"github.com/viant/agently/genai/executor/config"
)

// TestTurnPreviewLimit_NoAging verifies that when AgedAfterSteps or
// AgedLimit are not set, all turns use the primary Limit.
func TestTurnPreviewLimit_NoAging(t *testing.T) {
	s := &Service{defaults: &config.Defaults{PreviewSettings: config.PreviewSettings{
		Limit:          100,
		AgedAfterSteps: 0,
		AgedLimit:      0,
	}}}

	for turnIdx := 0; turnIdx < 5; turnIdx++ {
		got := s.turnPreviewLimit(turnIdx, 5, true)
		if got != 100 {
			t.Fatalf("expected limit=100 for turn %d with aging disabled, got %d", turnIdx, got)
		}
	}
}

// TestTurnPreviewLimit_AgingWindow verifies that only turns older than
// AgedAfterSteps are clamped to AgedLimit, and no aging happens when the
// total number of turns is within the window.
func TestTurnPreviewLimit_AgingWindow(t *testing.T) {
	s := &Service{defaults: &config.Defaults{PreviewSettings: config.PreviewSettings{
		Limit:          100,
		AgedAfterSteps: 2,
		AgedLimit:      10,
	}}}

	// totalTurns <= AgedAfterSteps: no aging; all use Limit.
	if got := s.turnPreviewLimit(0, 1, true); got != 100 {
		t.Fatalf("expected limit=100 for turn 0/1, got %d", got)
	}
	if got := s.turnPreviewLimit(1, 2, true); got != 100 {
		t.Fatalf("expected limit=100 for turn 1/2, got %d", got)
	}

	// totalTurns = 3, AgedAfterSteps=2: oldest turn (0) aged; turns 1 and 2 use Limit.
	if got := s.turnPreviewLimit(0, 3, true); got != 10 {
		t.Fatalf("expected agedLimit=10 for turn 0/3, got %d", got)
	}
	if got := s.turnPreviewLimit(1, 3, true); got != 100 {
		t.Fatalf("expected limit=100 for turn 1/3, got %d", got)
	}
	if got := s.turnPreviewLimit(2, 3, true); got != 100 {
		t.Fatalf("expected limit=100 for turn 2/3, got %d", got)
	}

	// applyAging=false: always use Limit regardless of turnIdx/totalTurns.
	if got := s.turnPreviewLimit(0, 3, false); got != 100 {
		t.Fatalf("expected limit=100 for turn 0/3 with applyAging=false, got %d", got)
	}
	if got := s.turnPreviewLimit(2, 3, false); got != 100 {
		t.Fatalf("expected limit=100 for turn 2/3 with applyAging=false, got %d", got)
	}
}
