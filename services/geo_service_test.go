package services

import (
	"testing"
	"time"
)

func TestGeoServiceFeed(t *testing.T) {
	fixed := time.Date(2025, time.November, 5, 12, 0, 0, 0, time.UTC)
	service := NewGeoService(fixed)

	feed := service.Feed()

	if feed.Source != geoSource {
		t.Fatalf("expected source %s, got %s", geoSource, feed.Source)
	}

	if !feed.GeneratedAt.Equal(fixed) {
		t.Fatalf("expected generated_at %s, got %s", fixed, feed.GeneratedAt)
	}

	if len(feed.Answers) == 0 {
		t.Fatal("expected at least 1 answer, got 0")
	}

	for idx, ans := range feed.Answers {
		if ans.Question == "" {
			t.Fatalf("answer %d missing question", idx)
		}
		if len(ans.Steps) == 0 {
			t.Fatalf("answer %d missing steps", idx)
		}
		if ans.CTA == "" {
			t.Fatalf("answer %d missing CTA", idx)
		}
	}

	feed.Answers[0].Question = "mutated"
	feed.Answers[0].Steps[0] = "changed"
	if feed.Answers[0].Question == service.answers[0].Question {
		t.Fatal("service internal state mutated by question edit")
	}
	if feed.Answers[0].Steps[0] == service.answers[0].Steps[0] {
		t.Fatal("service internal state mutated by step edit")
	}
}
