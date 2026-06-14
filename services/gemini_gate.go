package services

import (
	"context"
	"os"
	"strconv"
	"time"
)

// Gemini Flash on "standard" service tier rejects requests past ~6
// concurrent in-flight per project (measured via burst diagnostic endpoint).
// Default cap 4 leaves headroom for spikes below that ceiling.
// Override via GEMINI_CONCURRENCY env var (1-32).
//
// The gate is shared across all Gemini generation calls (Flash, Pro, tools)
// because they hit the same concurrency pool at Google's edge. Embedding has
// its own pool server-side and doesn't need this gate.
const defaultGeminiConcurrency = 4

// acquireSlotTimeout is the max time a caller will wait for a free slot.
// If all slots are busy for longer than this, we return an error rather than
// blocking indefinitely — callers should surface that as a rate-limit-style
// failure so the existing retry/UX paths trigger.
const acquireSlotTimeout = 10 * time.Second

var geminiGate = newGeminiGate(geminiConcurrencyFromEnv())

type gate struct {
	slots chan struct{}
}

func newGeminiGate(capacity int) *gate {
	return &gate{slots: make(chan struct{}, capacity)}
}

// acquire blocks until a slot is free, the context is cancelled, or
// acquireSlotTimeout elapses. Returns an error with "rate" in the message
// on timeout so IsRateLimitErr() classifies it correctly.
func (g *gate) acquire(ctx context.Context) error {
	timer := time.NewTimer(acquireSlotTimeout)
	defer timer.Stop()
	select {
	case g.slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return &gateTimeoutErr{}
	}
}

func (g *gate) release() {
	<-g.slots
}

type gateTimeoutErr struct{}

func (e *gateTimeoutErr) Error() string {
	return "gemini gate: rate limit — no concurrent slot available within timeout"
}

func geminiConcurrencyFromEnv() int {
	if v := os.Getenv("GEMINI_CONCURRENCY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 32 {
			return n
		}
	}
	return defaultGeminiConcurrency
}

// AcquireGeminiSlot is exported so callers outside this package (if any)
// can gate raw Gemini calls too. Most callers should just use the
// services.CallGemini* functions which gate internally.
func AcquireGeminiSlot(ctx context.Context) error {
	return geminiGate.acquire(ctx)
}

// ReleaseGeminiSlot releases a slot acquired via AcquireGeminiSlot.
// Must be paired with every successful Acquire.
func ReleaseGeminiSlot() {
	geminiGate.release()
}
