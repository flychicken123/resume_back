package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetAppConfig_Defaults(t *testing.T) {
	t.Setenv("GEMINI_RETRY_ENABLED", "")
	t.Setenv("CLASSIFY_PER_JOB_DELAY_MS", "")
	t.Setenv("CLASSIFY_BATCH_SIZE", "")

	cfg := GetAppConfig()
	assert.True(t, cfg.GeminiRetryEnabled)
	assert.Equal(t, 500, cfg.ClassifyPerJobDelayMS)
	assert.Equal(t, 5, cfg.ClassifyBatchSize)
}

func TestGetAppConfig_OverrideBooleanFalse(t *testing.T) {
	t.Setenv("GEMINI_RETRY_ENABLED", "false")
	cfg := GetAppConfig()
	assert.False(t, cfg.GeminiRetryEnabled)
}

func TestGetAppConfig_OverrideBooleanTrue(t *testing.T) {
	t.Setenv("GEMINI_RETRY_ENABLED", "true")
	cfg := GetAppConfig()
	assert.True(t, cfg.GeminiRetryEnabled)
}

func TestGetAppConfig_OverrideIntegers(t *testing.T) {
	t.Setenv("CLASSIFY_PER_JOB_DELAY_MS", "1500")
	t.Setenv("CLASSIFY_BATCH_SIZE", "10")
	cfg := GetAppConfig()
	assert.Equal(t, 1500, cfg.ClassifyPerJobDelayMS)
	assert.Equal(t, 10, cfg.ClassifyBatchSize)
}

func TestGetAppConfig_InvalidInt_FallsBack(t *testing.T) {
	t.Setenv("CLASSIFY_BATCH_SIZE", "abc")
	t.Setenv("CLASSIFY_PER_JOB_DELAY_MS", "xyz")
	cfg := GetAppConfig()
	assert.Equal(t, 5, cfg.ClassifyBatchSize)
	assert.Equal(t, 500, cfg.ClassifyPerJobDelayMS)
}

func TestGetAppConfig_NegativeInt_FallsBack(t *testing.T) {
	t.Setenv("CLASSIFY_BATCH_SIZE", "-3")
	cfg := GetAppConfig()
	assert.Equal(t, 5, cfg.ClassifyBatchSize)
}

func TestGetAppConfig_ZeroInt_FallsBack(t *testing.T) {
	t.Setenv("CLASSIFY_BATCH_SIZE", "0")
	t.Setenv("CLASSIFY_PER_JOB_DELAY_MS", "0")
	cfg := GetAppConfig()
	assert.Equal(t, 5, cfg.ClassifyBatchSize)
	assert.Equal(t, 500, cfg.ClassifyPerJobDelayMS)
}

func TestGetAppConfig_BooleanInvalid_FallsBackToDefault(t *testing.T) {
	t.Setenv("GEMINI_RETRY_ENABLED", "maybe")
	cfg := GetAppConfig()
	assert.True(t, cfg.GeminiRetryEnabled)
}
