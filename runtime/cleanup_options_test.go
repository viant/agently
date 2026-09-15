package runtime

import (
	"strings"
	"testing"
	"time"
)

var conversationCleanupEnvNames = []string{
	"AGENTLY_CLEANUP_ENABLED",
	"AGENTLY_CLEANUP_INTERVAL",
	"AGENTLY_CLEANUP_BATCH_SIZE",
	"AGENTLY_CLEANUP_TIMEOUT",
	"AGENTLY_CLEANUP_RUN_ON_START",
	"AGENTLY_CLEANUP_DEBUG",
	"AGENTLY_CLEANUP_INTERACTIVE_MODE",
	"AGENTLY_CLEANUP_INTERACTIVE_RETENTION_DAYS",
	"AGENTLY_CLEANUP_SCHEDULED_MODE",
	"AGENTLY_CLEANUP_SCHEDULED_RETENTION_DAYS",
	"AGENTLY_CLEANUP_ORPHAN_MODE",
	"AGENTLY_CLEANUP_ORPHAN_MIN_AGE_DAYS",
}

func TestConversationCleanupOptionsFromEnvDefaultsAreSafe(t *testing.T) {
	clearConversationCleanupEnv(t)

	got, err := ConversationCleanupOptionsFromEnv()
	if err != nil {
		t.Fatalf("ConversationCleanupOptionsFromEnv() error: %v", err)
	}
	if got.Enabled || got.RunOnStart || got.Debug {
		t.Fatalf("unsafe enabled default: %#v", got)
	}
	if got.InteractiveMode != ConversationCleanupModeOff || got.ScheduledMode != ConversationCleanupModeOff || got.OrphanMode != ConversationCleanupModeOff {
		t.Fatalf("cleanup policies must default to off: %#v", got)
	}
	if got.Interval != time.Hour || got.BatchSize != 50 || got.Timeout != 5*time.Minute || got.InteractiveRetention != 30*24*time.Hour || got.ScheduledRetention != 30*24*time.Hour || got.OrphanMinAge != 14*24*time.Hour {
		t.Fatalf("unexpected defaults: %#v", got)
	}
}

func TestConversationCleanupOptionsFromEnvParsesValues(t *testing.T) {
	clearConversationCleanupEnv(t)
	t.Setenv("AGENTLY_CLEANUP_ENABLED", "true")
	t.Setenv("AGENTLY_CLEANUP_INTERVAL", "30m")
	t.Setenv("AGENTLY_CLEANUP_BATCH_SIZE", "17")
	t.Setenv("AGENTLY_CLEANUP_TIMEOUT", "45s")
	t.Setenv("AGENTLY_CLEANUP_RUN_ON_START", "yes")
	t.Setenv("AGENTLY_CLEANUP_DEBUG", "1")
	t.Setenv("AGENTLY_CLEANUP_INTERACTIVE_MODE", "DRY-RUN")
	t.Setenv("AGENTLY_CLEANUP_INTERACTIVE_RETENTION_DAYS", "45")
	t.Setenv("AGENTLY_CLEANUP_SCHEDULED_MODE", "execute")
	t.Setenv("AGENTLY_CLEANUP_SCHEDULED_RETENTION_DAYS", "60")
	t.Setenv("AGENTLY_CLEANUP_ORPHAN_MODE", "dry-run")
	t.Setenv("AGENTLY_CLEANUP_ORPHAN_MIN_AGE_DAYS", "14")

	got, err := ConversationCleanupOptionsFromEnv()
	if err != nil {
		t.Fatalf("ConversationCleanupOptionsFromEnv() error: %v", err)
	}
	if !got.Enabled || !got.RunOnStart || !got.Debug {
		t.Fatalf("enabled values were not parsed: %#v", got)
	}
	if got.InteractiveMode != ConversationCleanupModeDryRun || got.ScheduledMode != ConversationCleanupModeExecute || got.OrphanMode != ConversationCleanupModeDryRun {
		t.Fatalf("cleanup modes were not parsed: %#v", got)
	}
	if got.Interval != 30*time.Minute || got.BatchSize != 17 || got.Timeout != 45*time.Second || got.InteractiveRetention != 45*24*time.Hour || got.ScheduledRetention != 60*24*time.Hour || got.OrphanMinAge != 14*24*time.Hour {
		t.Fatalf("unexpected parsed values: %#v", got)
	}
	if !got.InteractiveMode.Enabled() || got.InteractiveMode.Executes() || !got.ScheduledMode.Executes() {
		t.Fatalf("unexpected mode behavior: %#v", got)
	}
}

func TestConversationCleanupOptionsFromEnvRejectsInvalidExplicitValues(t *testing.T) {
	clearConversationCleanupEnv(t)
	t.Setenv("AGENTLY_CLEANUP_ENABLED", "sometimes")
	t.Setenv("AGENTLY_CLEANUP_INTERVAL", "0s")
	t.Setenv("AGENTLY_CLEANUP_BATCH_SIZE", "-2")
	t.Setenv("AGENTLY_CLEANUP_TIMEOUT", "not-a-duration")
	t.Setenv("AGENTLY_CLEANUP_RUN_ON_START", "perhaps")
	t.Setenv("AGENTLY_CLEANUP_INTERACTIVE_MODE", "apply")
	t.Setenv("AGENTLY_CLEANUP_INTERACTIVE_RETENTION_DAYS", "999999999999999999")
	t.Setenv("AGENTLY_CLEANUP_SCHEDULED_MODE", "delete")
	t.Setenv("AGENTLY_CLEANUP_SCHEDULED_RETENTION_DAYS", "0")
	t.Setenv("AGENTLY_CLEANUP_ORPHAN_MODE", "report")
	t.Setenv("AGENTLY_CLEANUP_ORPHAN_MIN_AGE_DAYS", "0")

	got, err := ConversationCleanupOptionsFromEnv()
	if err == nil {
		t.Fatal("ConversationCleanupOptionsFromEnv() error = nil")
	}
	for _, name := range []string{
		"AGENTLY_CLEANUP_ENABLED",
		"AGENTLY_CLEANUP_INTERVAL",
		"AGENTLY_CLEANUP_BATCH_SIZE",
		"AGENTLY_CLEANUP_TIMEOUT",
		"AGENTLY_CLEANUP_RUN_ON_START",
		"AGENTLY_CLEANUP_INTERACTIVE_MODE",
		"AGENTLY_CLEANUP_INTERACTIVE_RETENTION_DAYS",
		"AGENTLY_CLEANUP_SCHEDULED_MODE",
		"AGENTLY_CLEANUP_SCHEDULED_RETENTION_DAYS",
		"AGENTLY_CLEANUP_ORPHAN_MODE",
		"AGENTLY_CLEANUP_ORPHAN_MIN_AGE_DAYS",
	} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error %q does not mention %s", err, name)
		}
	}
	if got.Enabled || got.Interval != time.Hour || got.BatchSize != 50 || got.Timeout != 5*time.Minute || got.InteractiveMode != ConversationCleanupModeOff || got.ScheduledMode != ConversationCleanupModeOff || got.OrphanMode != ConversationCleanupModeOff {
		t.Fatalf("invalid values must not create unsafe options: %#v", got)
	}
}

func TestConversationCleanupOptionsFromEnvDoesNotUseLegacyAliases(t *testing.T) {
	clearConversationCleanupEnv(t)
	for _, name := range []string{
		"AGENTLY_CLEANUP_WORKER_ENABLED",
		"AGENTLY_CLEANUP_INTERACTIVE_ENABLED",
		"AGENTLY_CLEANUP_INTERACTIVE_DELETE_ENABLED",
		"AGENTLY_SCHEDULED_RETENTION_ENABLED",
		"AGENTLY_SCHEDULED_RETENTION_DRY_RUN",
		"AGENTLY_ORPHAN_CLEANUP_ENABLED",
		"AGENTLY_ORPHAN_CLEANUP_DRY_RUN",
		"AGENTLY_CLEANUP_ORPHAN_MIN_AGE",
	} {
		t.Setenv(name, "true")
	}

	got, err := ConversationCleanupOptionsFromEnv()
	if err != nil {
		t.Fatalf("ConversationCleanupOptionsFromEnv() error: %v", err)
	}
	if got.Enabled || got.InteractiveMode.Enabled() || got.ScheduledMode.Enabled() || got.OrphanMode.Enabled() {
		t.Fatalf("legacy aliases unexpectedly enabled cleanup: %#v", got)
	}
}

func clearConversationCleanupEnv(t *testing.T) {
	t.Helper()
	for _, name := range conversationCleanupEnvNames {
		t.Setenv(name, "")
	}
}
