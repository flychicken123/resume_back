package services

import (
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	JobSyncSchedulerDisabledEnv        = "DISABLE_JOB_SYNC_SCHEDULER"
	JobSyncSchedulerIntervalMinutesEnv = "JOB_SYNC_SCHEDULER_INTERVAL_MINUTES"
	MinJobSyncIntervalMinutesEnv       = "MIN_JOB_SYNC_INTERVAL_MINUTES"
)

// JobSyncSchedulerDisabled reports whether scheduled company job sync is disabled.
func JobSyncSchedulerDisabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(JobSyncSchedulerDisabledEnv)), "true")
}

// JobSyncSchedulerIntervalFromEnv returns how often the scheduler should check
// for due companies. It defaults to the configured minimum company sync interval.
func JobSyncSchedulerIntervalFromEnv() time.Duration {
	return durationMinutesFromEnv(JobSyncSchedulerIntervalMinutesEnv, MinimumJobSyncIntervalFromEnv())
}

// MinimumJobSyncIntervalFromEnv returns the minimum effective interval between
// syncs for one company. It defaults to 24 hours to preserve existing behavior.
func MinimumJobSyncIntervalFromEnv() time.Duration {
	return durationMinutesFromEnv(MinJobSyncIntervalMinutesEnv, dailySyncInterval)
}

func durationMinutesFromEnv(name string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}

	minutes, err := strconv.Atoi(raw)
	if err != nil || minutes <= 0 {
		return fallback
	}
	return time.Duration(minutes) * time.Minute
}

func effectiveCompanySyncInterval(syncIntervalMinutes int, minimum time.Duration) time.Duration {
	if minimum <= 0 {
		minimum = dailySyncInterval
	}

	interval := dailySyncInterval
	if syncIntervalMinutes > 0 {
		interval = time.Duration(syncIntervalMinutes) * time.Minute
	}
	if interval < minimum {
		return minimum
	}
	return interval
}
