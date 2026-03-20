package v1

import "testing"

func TestSchedulerOptionsFromEnv_Defaults(t *testing.T) {
	t.Setenv("AGENTLY_SCHEDULER_API", "")
	t.Setenv("AGENTLY_SCHEDULER_RUN_NOW", "")
	t.Setenv("AGENTLY_SCHEDULER_RUNNER", "")

	got := schedulerOptionsFromEnv()

	if !got.EnableAPI {
		t.Fatalf("expected EnableAPI default true")
	}
	if !got.EnableRunNow {
		t.Fatalf("expected EnableRunNow default true")
	}
	if got.EnableWatchdog {
		t.Fatalf("expected EnableWatchdog default false")
	}
}

func TestSchedulerOptionsFromEnv_DisableAPIAndRunNow(t *testing.T) {
	t.Setenv("AGENTLY_SCHEDULER_API", "off")
	t.Setenv("AGENTLY_SCHEDULER_RUN_NOW", "0")
	t.Setenv("AGENTLY_SCHEDULER_RUNNER", "")

	got := schedulerOptionsFromEnv()

	if got.EnableAPI {
		t.Fatalf("expected EnableAPI false")
	}
	if got.EnableRunNow {
		t.Fatalf("expected EnableRunNow false")
	}
	if got.EnableWatchdog {
		t.Fatalf("expected EnableWatchdog false")
	}
}

func TestSchedulerOptionsFromEnv_EnableRunnerAliases(t *testing.T) {
	t.Setenv("AGENTLY_SCHEDULER_API", "yes")
	t.Setenv("AGENTLY_SCHEDULER_RUN_NOW", "on")
	t.Setenv("AGENTLY_SCHEDULER_RUNNER", "true")

	got := schedulerOptionsFromEnv()

	if !got.EnableAPI {
		t.Fatalf("expected EnableAPI true")
	}
	if !got.EnableRunNow {
		t.Fatalf("expected EnableRunNow true")
	}
	if !got.EnableWatchdog {
		t.Fatalf("expected EnableWatchdog true")
	}
}
