package services

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type capturedLog struct {
	msg    string
	fields map[string]interface{}
}

type captureLogger struct {
	mu   sync.Mutex
	logs []capturedLog
}

func (c *captureLogger) Warn(msg string, data ...interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var fields map[string]interface{}
	if len(data) > 0 {
		fields, _ = data[0].(map[string]interface{})
	}
	c.logs = append(c.logs, capturedLog{msg: msg, fields: fields})
}

func (c *captureLogger) find(msg string) *capturedLog {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.logs {
		if c.logs[i].msg == msg {
			return &c.logs[i]
		}
	}
	return nil
}

func withFakeSleep(t *testing.T) *[]time.Duration {
	t.Helper()
	orig := retrySleep
	recorded := []time.Duration{}
	retrySleep = func(d time.Duration) { recorded = append(recorded, d) }
	t.Cleanup(func() { retrySleep = orig })
	return &recorded
}

func TestRetry_SucceedsOnFirstAttempt(t *testing.T) {
	calls := 0
	got, err := CallWithRateLimitRetry(context.Background(), DefaultRetryConfig(),
		func(ctx context.Context) (int, error) { calls++; return 42, nil },
		RetryOpts{Endpoint: "test"},
	)
	assert.NoError(t, err)
	assert.Equal(t, 42, got)
	assert.Equal(t, 1, calls)
}

func TestRetry_SucceedsOnSecondAttempt(t *testing.T) {
	withFakeSleep(t)
	calls := 0
	got, err := CallWithRateLimitRetry(context.Background(), DefaultRetryConfig(),
		func(ctx context.Context) (int, error) {
			calls++
			if calls < 2 {
				return 0, errors.New("429 too many requests")
			}
			return 7, nil
		},
		RetryOpts{Endpoint: "test"},
	)
	assert.NoError(t, err)
	assert.Equal(t, 7, got)
	assert.Equal(t, 2, calls)
}

func TestRetry_SucceedsOnThirdAttempt(t *testing.T) {
	withFakeSleep(t)
	calls := 0
	_, err := CallWithRateLimitRetry(context.Background(), DefaultRetryConfig(),
		func(ctx context.Context) (int, error) {
			calls++
			if calls < 3 {
				return 0, errors.New("ResourceExhausted")
			}
			return 1, nil
		},
		RetryOpts{Endpoint: "test"},
	)
	assert.NoError(t, err)
	assert.Equal(t, 3, calls)
}

func TestRetry_ExhaustsRetries_ReturnsLastError(t *testing.T) {
	withFakeSleep(t)
	calls := 0
	_, err := CallWithRateLimitRetry(context.Background(), DefaultRetryConfig(),
		func(ctx context.Context) (int, error) {
			calls++
			return 0, errors.New("429 attempt")
		},
		RetryOpts{Endpoint: "test"},
	)
	assert.Error(t, err)
	assert.Equal(t, 3, calls)
	assert.True(t, IsRateLimitErr(err))
}

func TestRetry_NonRateLimitError_FailsFast(t *testing.T) {
	calls := 0
	_, err := CallWithRateLimitRetry(context.Background(), DefaultRetryConfig(),
		func(ctx context.Context) (int, error) {
			calls++
			return 0, errors.New("unrelated boom")
		},
		RetryOpts{Endpoint: "test"},
	)
	assert.Error(t, err)
	assert.Equal(t, 1, calls)
	assert.False(t, IsRateLimitErr(err))
}

func TestRetry_ContextCancelled_BeforeFirstCall(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	_, err := CallWithRateLimitRetry(ctx, DefaultRetryConfig(),
		func(ctx context.Context) (int, error) { calls++; return 0, nil },
		RetryOpts{Endpoint: "test"},
	)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 0, calls)
}

func TestRetry_ContextCancelled_DuringBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	calls := 0
	_, err := CallWithRateLimitRetry(ctx, DefaultRetryConfig(),
		func(ctx context.Context) (int, error) {
			calls++
			return 0, errors.New("429")
		},
		RetryOpts{Endpoint: "test"},
	)
	elapsed := time.Since(start)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, calls)
	assert.Less(t, elapsed, 500*time.Millisecond)
}

func TestRetry_ExponentialBackoff_TimingBudget(t *testing.T) {
	recorded := withFakeSleep(t)
	_, _ = CallWithRateLimitRetry(context.Background(), DefaultRetryConfig(),
		func(ctx context.Context) (int, error) { return 0, errors.New("429") },
		RetryOpts{Endpoint: "test"},
	)
	assert.Equal(t, []time.Duration{1 * time.Second, 2 * time.Second}, *recorded)
}

func TestRetry_FeatureFlagDisabled_RunsOnce(t *testing.T) {
	t.Setenv("GEMINI_RETRY_ENABLED", "false")
	calls := 0
	_, err := CallWithRateLimitRetry(context.Background(), DefaultRetryConfig(),
		func(ctx context.Context) (int, error) {
			calls++
			return 0, errors.New("429")
		},
		RetryOpts{Endpoint: "test"},
	)
	assert.Error(t, err)
	assert.Equal(t, 1, calls)
}

func TestRetry_FeatureFlagEnabled_RetriesAsNormal(t *testing.T) {
	t.Setenv("GEMINI_RETRY_ENABLED", "true")
	withFakeSleep(t)
	calls := 0
	_, _ = CallWithRateLimitRetry(context.Background(), DefaultRetryConfig(),
		func(ctx context.Context) (int, error) { calls++; return 0, errors.New("429") },
		RetryOpts{Endpoint: "test"},
	)
	assert.Equal(t, 3, calls)
}

func TestRetry_FeatureFlagUnset_DefaultsToEnabled(t *testing.T) {
	t.Setenv("GEMINI_RETRY_ENABLED", "")
	withFakeSleep(t)
	calls := 0
	_, err := CallWithRateLimitRetry(context.Background(), DefaultRetryConfig(),
		func(ctx context.Context) (int, error) {
			calls++
			if calls < 2 {
				return 0, errors.New("429")
			}
			return 99, nil
		},
		RetryOpts{Endpoint: "test"},
	)
	assert.NoError(t, err)
	assert.Equal(t, 2, calls)
}

func TestRetry_GenericReturnType_Struct(t *testing.T) {
	type payload struct{ Name string }
	got, err := CallWithRateLimitRetry(context.Background(), DefaultRetryConfig(),
		func(ctx context.Context) (payload, error) {
			return payload{Name: "ok"}, nil
		},
		RetryOpts{Endpoint: "test"},
	)
	assert.NoError(t, err)
	assert.Equal(t, "ok", got.Name)
}

func TestRetry_LogEmitted_OnRetry(t *testing.T) {
	withFakeSleep(t)
	lg := &captureLogger{}
	calls := 0
	_, _ = CallWithRateLimitRetry(context.Background(), DefaultRetryConfig(),
		func(ctx context.Context) (int, error) {
			calls++
			if calls < 2 {
				return 0, errors.New("429")
			}
			return 1, nil
		},
		RetryOpts{Endpoint: "chat", Logger: lg},
	)
	log := lg.find("gemini_rate_limit_retry")
	assert.NotNil(t, log)
	if log != nil {
		assert.Equal(t, "chat", log.fields["endpoint"])
		assert.Equal(t, 2, log.fields["attempt"])
		assert.Equal(t, int64(1000), log.fields["backoff_ms"])
	}
}

func TestRetry_LogEmitted_OnExhaustion(t *testing.T) {
	withFakeSleep(t)
	lg := &captureLogger{}
	_, _ = CallWithRateLimitRetry(context.Background(), DefaultRetryConfig(),
		func(ctx context.Context) (int, error) { return 0, errors.New("429") },
		RetryOpts{Endpoint: "chat", Logger: lg},
	)
	log := lg.find("gemini_rate_limit_exhausted")
	assert.NotNil(t, log)
	if log != nil {
		assert.Equal(t, "chat", log.fields["endpoint"])
		assert.Equal(t, int64(3000), log.fields["total_wait_ms"])
	}
}
