package runtime

import (
	"os"
	"strconv"
	"strings"

	"github.com/viant/agently-core/sdk"
)

func SchedulerOptionsFromEnv() *sdk.SchedulerOptions {
	return &sdk.SchedulerOptions{
		EnableAPI:      envEnabledDefaultTrue("AGENTLY_SCHEDULER_API"),
		EnableRunNow:   envEnabledDefaultTrue("AGENTLY_SCHEDULER_RUN_NOW"),
		EnableWatchdog: envEnabledDefaultFalse("AGENTLY_SCHEDULER_RUNNER"),
	}
}

// SchedulerMaxConcurrentRunsFromEnv reads AGENTLY_SCHEDULER_MAX_CONCURRENT_RUNS
// and returns the configured cap, or 0 (unbounded) when unset / invalid /
// non-positive. Serve threads the value into the scheduler service so a burst
// of due schedules cannot explode into thousands of parallel goroutines.
func SchedulerMaxConcurrentRunsFromEnv() int {
	raw := strings.TrimSpace(os.Getenv("AGENTLY_SCHEDULER_MAX_CONCURRENT_RUNS"))
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

func envEnabledDefaultTrue(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "0", "false", "no", "n", "off":
		return false
	default:
		return true
	}
}

func envEnabledDefaultFalse(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}
