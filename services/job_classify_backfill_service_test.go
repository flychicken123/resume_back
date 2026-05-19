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

func TestBackfillStatus_IncludesRecentErrorLogs(t *testing.T) {
	svc := NewJobClassifyBackfillService(nil, utils.NewLogger())

	svc.recordError(123, "failed to classify job")

	status := svc.Status()
	assert.Equal(t, 1, status.Errors)
	assert.Len(t, status.ErrorLogs, 1)
	assert.Equal(t, int64(123), status.ErrorLogs[0].JobID)
	assert.Equal(t, "failed to classify job", status.ErrorLogs[0].Message)
	assert.False(t, status.ErrorLogs[0].At.IsZero())
}

func TestBackfillStatus_ErrorLogsAreDefensiveCopy(t *testing.T) {
	svc := NewJobClassifyBackfillService(nil, utils.NewLogger())
	svc.recordError(123, "original")

	status := svc.Status()
	status.ErrorLogs[0].Message = "mutated"

	assert.Equal(t, "original", svc.Status().ErrorLogs[0].Message)
}
