package services

import (
	"context"
	"errors"
	"testing"
)

func TestTriggerReturnsErrEmbeddingSvcNotConfigured(t *testing.T) {
	svc := NewJobEmbeddingBackfillService(nil, nil, nil)
	err := svc.Trigger(context.Background(), 100, 0)
	if !errors.Is(err, ErrEmbeddingSvcNotConfigured) {
		t.Fatalf("expected ErrEmbeddingSvcNotConfigured, got %v", err)
	}
}

func TestTriggerReturnsErrAlreadyRunning(t *testing.T) {
	svc := NewJobEmbeddingBackfillService(nil, &EmbeddingService{}, nil)
	svc.mu.Lock()
	svc.status.Running = true
	svc.mu.Unlock()

	err := svc.Trigger(context.Background(), 100, 0)
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("expected ErrAlreadyRunning, got %v", err)
	}
}

func TestStatusInitialState(t *testing.T) {
	svc := NewJobEmbeddingBackfillService(nil, nil, nil)
	s := svc.Status()
	if s.Running {
		t.Error("expected Running=false for fresh service")
	}
	if s.Processed != 0 {
		t.Errorf("expected Processed=0, got %d", s.Processed)
	}
	if s.Errors != 0 {
		t.Errorf("expected Errors=0, got %d", s.Errors)
	}
	if s.Total != 0 {
		t.Errorf("expected Total=0, got %d", s.Total)
	}
	if s.StartedAt != nil {
		t.Errorf("expected StartedAt=nil, got %v", s.StartedAt)
	}
	if s.FinishedAt != nil {
		t.Errorf("expected FinishedAt=nil, got %v", s.FinishedAt)
	}
}

func TestStatusReturnsCopy(t *testing.T) {
	svc := NewJobEmbeddingBackfillService(nil, nil, nil)
	snap := svc.Status()
	snap.Processed = 999
	snap.Running = true

	second := svc.Status()
	if second.Processed != 0 {
		t.Errorf("mutating returned status should not affect internal state: got Processed=%d", second.Processed)
	}
	if second.Running {
		t.Error("mutating returned status should not affect internal state: got Running=true")
	}
}
