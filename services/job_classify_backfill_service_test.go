package services

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"resumeai/utils"
)

func TestBackfillCancel_NoActiveRun_ReturnsFalse(t *testing.T) {
	svc := NewJobClassifyBackfillService(nil, utils.NewLogger())
	assert.False(t, svc.Cancel())
}

func TestBackfillCancel_ActiveRun_ReturnsTrue(t *testing.T) {
	svc := NewJobClassifyBackfillService(nil, utils.NewLogger())
	ctx, cancel := context.WithCancel(context.Background())
	svc.setRunningForTest(ctx, cancel)

	assert.True(t, svc.Cancel())
	select {
	case <-ctx.Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected ctx cancelled after Cancel()")
	}
}

func TestBackfillCancel_Idempotent(t *testing.T) {
	svc := NewJobClassifyBackfillService(nil, utils.NewLogger())
	ctx, cancel := context.WithCancel(context.Background())
	svc.setRunningForTest(ctx, cancel)

	assert.True(t, svc.Cancel())
	assert.False(t, svc.Cancel())
}
