package services

import (
	"context"
	"os"
	"strings"
	"time"
)

type RetryConfig struct {
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
}

func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:    3,
		InitialBackoff: 1 * time.Second,
		MaxBackoff:     8 * time.Second,
	}
}

type retryLogger interface {
	Warn(msg string, data ...interface{})
}

type RetryOpts struct {
	Endpoint string
	Logger   retryLogger
}

// retrySleep is a package-private indirection so tests can swap real sleep for a recorder.
// Must not wrap with context — callers pass ctx separately.
var retrySleep = time.Sleep

// CallWithRateLimitRetry invokes fn, retrying on rate-limit errors only
// (per IsRateLimitErr) with exponential backoff. Non-rate-limit errors are
// returned immediately on first occurrence.
//
// Kill switch: if GEMINI_RETRY_ENABLED is set to "false", fn runs exactly once
// with no retries. Any other value (including unset) enables retries.
//
// MUST NOT be used around calls that stream bytes to the client — retries
// would corrupt partial output. Use only on pre-stream blocking calls.
func CallWithRateLimitRetry[T any](
	ctx context.Context,
	cfg RetryConfig,
	fn func(ctx context.Context) (T, error),
	opts RetryOpts,
) (T, error) {
	var zero T

	if err := ctx.Err(); err != nil {
		return zero, err
	}

	if !retryEnabled() {
		return fn(ctx)
	}

	backoff := cfg.InitialBackoff
	var lastErr error
	var totalWait time.Duration

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return zero, err
		}

		result, err := fn(ctx)
		if err == nil {
			return result, nil
		}
		lastErr = err

		if !IsRateLimitErr(err) {
			return zero, err
		}

		if attempt == cfg.MaxAttempts {
			break
		}

		if opts.Logger != nil {
			opts.Logger.Warn("gemini_rate_limit_retry", map[string]interface{}{
				"attempt":    attempt + 1,
				"backoff_ms": backoff.Milliseconds(),
				"endpoint":   opts.Endpoint,
			})
		}

		if err := sleepWithCtx(ctx, backoff); err != nil {
			return zero, err
		}
		totalWait += backoff

		backoff *= 2
		if backoff > cfg.MaxBackoff {
			backoff = cfg.MaxBackoff
		}
	}

	if opts.Logger != nil {
		opts.Logger.Warn("gemini_rate_limit_exhausted", map[string]interface{}{
			"endpoint":      opts.Endpoint,
			"total_wait_ms": totalWait.Milliseconds(),
		})
	}
	return zero, lastErr
}

func sleepWithCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	// Indirection through retrySleep lets tests observe/replace the sleep.
	done := make(chan struct{})
	go func() {
		retrySleep(d)
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func retryEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("GEMINI_RETRY_ENABLED")))
	return v != "false"
}
