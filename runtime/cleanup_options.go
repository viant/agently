package runtime

import (
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultConversationCleanupInterval                 = time.Hour
	defaultConversationCleanupBatchSize                = 50
	defaultConversationCleanupTimeout                  = 5 * time.Minute
	defaultConversationCleanupInteractiveRetentionDays = 30
	defaultConversationCleanupScheduledRetentionDays   = 30
	defaultConversationCleanupOrphanMinAgeDays         = 14
)

// ConversationCleanupMode controls whether a cleanup policy is disabled,
// validates and reports candidates, or executes its configured mutations.
type ConversationCleanupMode string

const (
	ConversationCleanupModeOff     ConversationCleanupMode = "off"
	ConversationCleanupModeDryRun  ConversationCleanupMode = "dry-run"
	ConversationCleanupModeExecute ConversationCleanupMode = "execute"
)

func (m ConversationCleanupMode) Enabled() bool {
	return m == ConversationCleanupModeDryRun || m == ConversationCleanupModeExecute
}

func (m ConversationCleanupMode) Executes() bool {
	return m == ConversationCleanupModeExecute
}

// ConversationCleanupOptions controls the maintenance worker and its policies.
// Cleanup passes are coordinated across application instances by a database
// maintenance lease. A local guard additionally prevents in-process overlap.
type ConversationCleanupOptions struct {
	Enabled              bool
	Interval             time.Duration
	BatchSize            int
	Timeout              time.Duration
	RunOnStart           bool
	Debug                bool
	InteractiveMode      ConversationCleanupMode
	InteractiveRetention time.Duration
	ScheduledMode        ConversationCleanupMode
	ScheduledRetention   time.Duration
	OrphanMode           ConversationCleanupMode
	OrphanMinAge         time.Duration
}

// ConversationCleanupOptionsFromEnv returns safe defaults for absent values
// and an error for every explicitly configured invalid value. It deliberately
// does not recognize the pre-release cleanup variable names.
func ConversationCleanupOptionsFromEnv() (ConversationCleanupOptions, error) {
	result := ConversationCleanupOptions{
		Interval:             defaultConversationCleanupInterval,
		BatchSize:            defaultConversationCleanupBatchSize,
		Timeout:              defaultConversationCleanupTimeout,
		InteractiveMode:      ConversationCleanupModeOff,
		InteractiveRetention: defaultConversationCleanupInteractiveRetentionDays * 24 * time.Hour,
		ScheduledMode:        ConversationCleanupModeOff,
		ScheduledRetention:   defaultConversationCleanupScheduledRetentionDays * 24 * time.Hour,
		OrphanMode:           ConversationCleanupModeOff,
		OrphanMinAge:         defaultConversationCleanupOrphanMinAgeDays * 24 * time.Hour,
	}

	var parseErrors []error
	assign := func(err error) {
		if err != nil {
			parseErrors = append(parseErrors, err)
		}
	}

	var err error
	result.Enabled, err = cleanupBoolFromEnv("AGENTLY_CLEANUP_ENABLED", false)
	assign(err)
	result.Interval, err = cleanupDurationFromEnv("AGENTLY_CLEANUP_INTERVAL", result.Interval)
	assign(err)
	result.BatchSize, err = cleanupPositiveIntFromEnv("AGENTLY_CLEANUP_BATCH_SIZE", result.BatchSize)
	assign(err)
	result.Timeout, err = cleanupDurationFromEnv("AGENTLY_CLEANUP_TIMEOUT", result.Timeout)
	assign(err)
	result.RunOnStart, err = cleanupBoolFromEnv("AGENTLY_CLEANUP_RUN_ON_START", false)
	assign(err)
	result.Debug, err = cleanupBoolFromEnv("AGENTLY_CLEANUP_DEBUG", false)
	assign(err)
	result.InteractiveMode, err = cleanupModeFromEnv("AGENTLY_CLEANUP_INTERACTIVE_MODE", result.InteractiveMode)
	assign(err)
	result.InteractiveRetention, err = cleanupDaysFromEnv(
		"AGENTLY_CLEANUP_INTERACTIVE_RETENTION_DAYS",
		defaultConversationCleanupInteractiveRetentionDays,
	)
	assign(err)
	result.ScheduledMode, err = cleanupModeFromEnv("AGENTLY_CLEANUP_SCHEDULED_MODE", result.ScheduledMode)
	assign(err)
	result.ScheduledRetention, err = cleanupDaysFromEnv(
		"AGENTLY_CLEANUP_SCHEDULED_RETENTION_DAYS",
		defaultConversationCleanupScheduledRetentionDays,
	)
	assign(err)
	result.OrphanMode, err = cleanupModeFromEnv("AGENTLY_CLEANUP_ORPHAN_MODE", result.OrphanMode)
	assign(err)
	result.OrphanMinAge, err = cleanupDaysFromEnv(
		"AGENTLY_CLEANUP_ORPHAN_MIN_AGE_DAYS",
		defaultConversationCleanupOrphanMinAgeDays,
	)
	assign(err)

	return result, errors.Join(parseErrors...)
}

func cleanupBoolFromEnv(name string, fallback bool) (bool, error) {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	if raw == "" {
		return fallback, nil
	}
	switch raw {
	case "1", "true", "yes", "y", "on":
		return true, nil
	case "0", "false", "no", "n", "off":
		return false, nil
	default:
		return fallback, fmt.Errorf("%s must be a boolean, got %q", name, raw)
	}
}

func cleanupModeFromEnv(name string, fallback ConversationCleanupMode) (ConversationCleanupMode, error) {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	if raw == "" {
		return fallback, nil
	}
	mode := ConversationCleanupMode(raw)
	switch mode {
	case ConversationCleanupModeOff, ConversationCleanupModeDryRun, ConversationCleanupModeExecute:
		return mode, nil
	default:
		return fallback, fmt.Errorf("%s must be one of off, dry-run, execute, got %q", name, raw)
	}
}

func cleanupDaysFromEnv(name string, fallbackDays int) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return time.Duration(fallbackDays) * 24 * time.Hour, nil
	}
	days, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || days <= 0 {
		return time.Duration(fallbackDays) * 24 * time.Hour, fmt.Errorf("%s must be a positive integer, got %q", name, raw)
	}
	const day = int64(24 * time.Hour)
	if days > math.MaxInt64/day {
		return time.Duration(fallbackDays) * 24 * time.Hour, fmt.Errorf("%s is too large, got %q", name, raw)
	}
	return time.Duration(days) * 24 * time.Hour, nil
}

func cleanupPositiveIntFromEnv(name string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseInt(raw, 10, strconv.IntSize)
	if err != nil || value <= 0 {
		return fallback, fmt.Errorf("%s must be a positive integer, got %q", name, raw)
	}
	return int(value), nil
}

func cleanupDurationFromEnv(name string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return fallback, fmt.Errorf("%s must be a positive duration, got %q", name, raw)
	}
	return value, nil
}
