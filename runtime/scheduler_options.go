package runtime

import (
	"os"
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
