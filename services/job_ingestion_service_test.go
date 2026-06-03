package services

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"resumeai/utils"
)

func newTestIngestionService(throttle ClassifyThrottleConfig) *JobIngestionService {
	return &JobIngestionService{
		logger:   utils.NewLogger(),
		throttle: throttle,
	}
}

func TestClassifierPauseFlag_DefaultFalse(t *testing.T) {
	svc := newTestIngestionService(ClassifyThrottleConfig{})
	assert.False(t, svc.IsClassifierPaused())
}

func TestClassifierPauseFlag_PauseAndResume(t *testing.T) {
	svc := newTestIngestionService(ClassifyThrottleConfig{})
	svc.PauseClassifier()
	assert.True(t, svc.IsClassifierPaused())
	svc.ResumeClassifier()
	assert.False(t, svc.IsClassifierPaused())
}

func TestClassifyLoop_PausedFlag_ExitsCleanly(t *testing.T) {
	svc := newTestIngestionService(ClassifyThrottleConfig{PerJobDelayMS: 0, BatchSize: 5})
	var processed atomic.Int32
	svc.classifyOneFn = func(ctx context.Context, id int64) error {
		processed.Add(1)
		svc.PauseClassifier()
		return nil
	}
	svc.classifyJobPostingsBatch(context.Background(), []int64{1, 2, 3, 4, 5})
	assert.Equal(t, int32(1), processed.Load())
}

func TestClassifyLoop_ContextCancelled_ExitsImmediately(t *testing.T) {
	svc := newTestIngestionService(ClassifyThrottleConfig{PerJobDelayMS: 0, BatchSize: 5})
	var processed atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	svc.classifyOneFn = func(c context.Context, id int64) error {
		processed.Add(1)
		if processed.Load() == 2 {
			cancel()
		}
		return nil
	}
	svc.classifyJobPostingsBatch(ctx, []int64{1, 2, 3, 4, 5})
	assert.Equal(t, int32(2), processed.Load())
}

func TestClassifyLoop_GlobalAIBackgroundKillSwitchDoesNotStopPulledJobClassification(t *testing.T) {
	t.Setenv("DISABLE_AI_BACKGROUND_JOBS", "true")
	t.Setenv(DisableJobAutoClassificationEnv, "false")

	svc := newTestIngestionService(ClassifyThrottleConfig{PerJobDelayMS: 0, BatchSize: 5})
	var processed atomic.Int32
	svc.classifyOneFn = func(ctx context.Context, id int64) error {
		processed.Add(1)
		return nil
	}

	svc.classifyJobPostingsBatch(context.Background(), []int64{1, 2, 3})

	assert.Equal(t, int32(3), processed.Load())
}

func TestClassifyLoop_JobAutoClassificationKillSwitchStopsPulledJobClassification(t *testing.T) {
	t.Setenv("DISABLE_AI_BACKGROUND_JOBS", "false")
	t.Setenv(DisableJobAutoClassificationEnv, "true")

	svc := newTestIngestionService(ClassifyThrottleConfig{PerJobDelayMS: 0, BatchSize: 5})
	var processed atomic.Int32
	svc.classifyOneFn = func(ctx context.Context, id int64) error {
		processed.Add(1)
		return nil
	}

	svc.classifyJobPostingsBatch(context.Background(), []int64{1, 2, 3})

	assert.Equal(t, int32(0), processed.Load())
}

func TestClassifyQueue_ProcessesQueuedIDs(t *testing.T) {
	t.Setenv(DisableJobAutoClassificationEnv, "false")

	svc := newTestIngestionService(ClassifyThrottleConfig{PerJobDelayMS: 0, BatchSize: 5})
	var processed atomic.Int32
	var idsMu sync.Mutex
	processedIDs := make([]int64, 0, 3)
	done := make(chan struct{})
	var closeDone sync.Once

	svc.classifyOneFn = func(ctx context.Context, id int64) error {
		idsMu.Lock()
		processedIDs = append(processedIDs, id)
		idsMu.Unlock()
		if processed.Add(1) == 3 {
			closeDone.Do(func() { close(done) })
		}
		return nil
	}

	svc.enqueueClassifyJobPostings([]int64{3, 1, 2})

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for classification queue")
	}

	idsMu.Lock()
	defer idsMu.Unlock()
	assert.Equal(t, []int64{3, 2, 1}, processedIDs)
}

func TestClassifyQueue_SkipsDuplicateQueuedAndInFlightIDs(t *testing.T) {
	t.Setenv(DisableJobAutoClassificationEnv, "false")

	svc := newTestIngestionService(ClassifyThrottleConfig{PerJobDelayMS: 0, BatchSize: 5})
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	var closeStarted sync.Once
	var closeDone sync.Once
	var processed atomic.Int32
	var idsMu sync.Mutex
	processedIDs := make([]int64, 0, 3)

	svc.classifyOneFn = func(ctx context.Context, id int64) error {
		idsMu.Lock()
		processedIDs = append(processedIDs, id)
		idsMu.Unlock()
		if id == 2 {
			closeStarted.Do(func() { close(started) })
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if processed.Add(1) == 3 {
			closeDone.Do(func() { close(done) })
		}
		return nil
	}

	svc.enqueueClassifyJobPostings([]int64{1, 2})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first classification")
	}

	svc.enqueueClassifyJobPostings([]int64{2, 3})
	close(release)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for classification queue")
	}

	idsMu.Lock()
	defer idsMu.Unlock()
	assert.Equal(t, []int64{2, 1, 3}, processedIDs)
}

func TestClassifyLoop_SleepsPerJob(t *testing.T) {
	svc := newTestIngestionService(ClassifyThrottleConfig{PerJobDelayMS: 123, BatchSize: 5})
	svc.classifyOneFn = func(ctx context.Context, id int64) error { return nil }

	start := time.Now()
	svc.classifyJobPostingsBatch(context.Background(), []int64{1, 2, 3})
	elapsed := time.Since(start)

	assert.GreaterOrEqual(t, elapsed, 300*time.Millisecond)
	assert.Less(t, elapsed, 1*time.Second)
}

func TestClassifyLoop_EmitsClassifyTickLog(t *testing.T) {
	lg := &captureLogger{}
	svc := newTestIngestionService(ClassifyThrottleConfig{PerJobDelayMS: 0, BatchSize: 5})
	svc.testLogger = lg
	svc.classifyOneFn = func(ctx context.Context, id int64) error { return nil }
	svc.classifyJobPostingsBatch(context.Background(), []int64{1, 2, 3})

	log := lg.find("classify_tick")
	assert.NotNil(t, log)
	if log != nil {
		assert.Equal(t, 5, log.fields["batch_size"])
		assert.Equal(t, 0, log.fields["delay_ms"])
		assert.Equal(t, 3, log.fields["remaining_backlog"])
	}
}

func TestClassifyLoop_SerializesConcurrentBatches(t *testing.T) {
	svc := newTestIngestionService(ClassifyThrottleConfig{PerJobDelayMS: 0, BatchSize: 5})
	var current atomic.Int32
	var maxSeen atomic.Int32

	svc.classifyOneFn = func(ctx context.Context, id int64) error {
		n := current.Add(1)
		for {
			max := maxSeen.Load()
			if n <= max || maxSeen.CompareAndSwap(max, n) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		current.Add(-1)
		return nil
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		svc.classifyJobPostingsBatch(context.Background(), []int64{1, 2, 3})
	}()
	go func() {
		defer wg.Done()
		svc.classifyJobPostingsBatch(context.Background(), []int64{4, 5, 6})
	}()
	wg.Wait()

	assert.Equal(t, int32(1), maxSeen.Load())
}
