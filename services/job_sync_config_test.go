package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestJobSyncSchedulerDisabled_UsesDedicatedEnv(t *testing.T) {
	t.Setenv(JobSyncSchedulerDisabledEnv, "true")
	t.Setenv("DISABLE_AI_BACKGROUND_JOBS", "false")

	assert.True(t, JobSyncSchedulerDisabled())
}

func TestJobSyncSchedulerDisabled_DoesNotUseAIKillSwitch(t *testing.T) {
	t.Setenv(JobSyncSchedulerDisabledEnv, "false")
	t.Setenv("DISABLE_AI_BACKGROUND_JOBS", "true")

	assert.False(t, JobSyncSchedulerDisabled())
}

func TestMinimumJobSyncIntervalFromEnv_DefaultsToTwentyFourHours(t *testing.T) {
	t.Setenv(MinJobSyncIntervalMinutesEnv, "")

	assert.Equal(t, 24*time.Hour, MinimumJobSyncIntervalFromEnv())
}

func TestMinimumJobSyncIntervalFromEnv_UsesMinutes(t *testing.T) {
	t.Setenv(MinJobSyncIntervalMinutesEnv, "180")

	assert.Equal(t, 3*time.Hour, MinimumJobSyncIntervalFromEnv())
}

func TestJobSyncSchedulerIntervalFromEnv_DefaultsToMinimumInterval(t *testing.T) {
	t.Setenv(MinJobSyncIntervalMinutesEnv, "180")
	t.Setenv(JobSyncSchedulerIntervalMinutesEnv, "")

	assert.Equal(t, 3*time.Hour, JobSyncSchedulerIntervalFromEnv())
}

func TestEffectiveCompanySyncInterval_UsesConfigurableMinimum(t *testing.T) {
	assert.Equal(t, 3*time.Hour, effectiveCompanySyncInterval(60, 3*time.Hour))
	assert.Equal(t, 4*time.Hour, effectiveCompanySyncInterval(240, 3*time.Hour))
}

func TestEffectiveCompanySyncInterval_NonPositiveCompanyIntervalDefaultsToTwentyFourHours(t *testing.T) {
	assert.Equal(t, 24*time.Hour, effectiveCompanySyncInterval(0, time.Hour))
}
